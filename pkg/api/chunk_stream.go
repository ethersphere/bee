// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/api/pb"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy/getter"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/gorilla/websocket"
)

const (
	streamReadTimeout = 15 * time.Minute

	// chunkDeliveryWriteDeadline bounds a single delivery write. It is
	// deliberately generous: a client that buffers ahead stops reading from the
	// socket while its buffer drains, and must not have its whole stream torn
	// down.
	chunkDeliveryWriteDeadline = 5 * time.Minute

	// chunkDownloadRequestTimeout bounds a single chunk retrieval.
	chunkDownloadRequestTimeout = getter.DefaultFetchTimeout

	// chunkUploadRequestTimeout bounds a single chunk upload/push.
	chunkUploadRequestTimeout = 30 * time.Second

	// chunkStreamCloseDeadline bounds a close frame write.
	chunkStreamCloseDeadline = 250 * time.Millisecond

	chunkStreamSubprotocol = "swarm-chunk-stream"

	defaultStreamSubWorkers = 8
	maxStreamQueueSize      = 1024
	maxStreamFrameSize      = 64 * 1024
)

var successWsMsg = []byte{}

func (s *Service) chunkStreamHandler(w http.ResponseWriter, r *http.Request) {
	// Reject plain HTTP requests before any tag, putter or header work is done.
	if !websocket.IsWebSocketUpgrade(r) {
		logger := s.logger.WithName("chunks_stream").Build()
		logger.Debug("chunk stream: not a websocket upgrade request")
		jsonhttp.BadRequest(w, "not a websocket upgrade request")
		return
	}

	mode := r.URL.Query().Get("mode")
	switch mode {
	case "", "stream", "upload":
	default:
		logger := s.logger.WithName("chunks_stream").Build()
		logger.Debug("chunk stream: invalid mode query parameter", "value", mode)
		jsonhttp.BadRequest(w, "invalid mode query parameter")
		return
	}

	subprotocols := websocket.Subprotocols(r)
	if slices.Contains(subprotocols, chunkStreamSubprotocol) || mode == "stream" {
		s.chunkBidirectionalStreamHandler(w, r)
		return
	}
	s.chunkUploadStreamHandler(w, r)
}

func (s *Service) chunkUploadStreamHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("chunks_stream").Build()

	headers := struct {
		BatchID  []byte `map:"Swarm-Postage-Batch-Id"` // Optional: omit if caller provides pre-signed stamps per chunk
		SwarmTag uint64 `map:"Swarm-Tag"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	// Fallback: read tag from query parameter (browser WebSocket can't set headers)
	if headers.SwarmTag == 0 {
		if qTag := r.URL.Query().Get("swarm-tag"); qTag != "" {
			parsed, err := strconv.ParseUint(qTag, 10, 64)
			if err != nil {
				logger.Debug("invalid swarm-tag query parameter", "value", qTag, "error", err)
				jsonhttp.BadRequest(w, "invalid swarm-tag query parameter")
				return
			}
			headers.SwarmTag = parsed
		}
	}

	var (
		tag uint64
		err error
	)
	if headers.SwarmTag > 0 {
		tag, err = s.getOrCreateSessionID(headers.SwarmTag)
		if err != nil {
			logger.Debug("get or create tag failed", "error", err)
			logger.Error(nil, "get or create tag failed")
			switch {
			case errors.Is(err, storage.ErrNotFound):
				jsonhttp.NotFound(w, "tag not found")
			default:
				jsonhttp.InternalServerError(w, "cannot get or create tag")
			}
			return
		}
	}

	// Create connection-level putter only if BatchID is provided.
	// If BatchID is not provided, the API caller is expected to provide
	// pre-signed stamps with each chunk (and is also expected to keep
	// track of stamp state over time).
	var putter storer.PutterSession
	if len(headers.BatchID) > 0 {
		// if tag not specified use direct upload
		// Using context.Background here because the putter's lifetime extends beyond that of the HTTP request.
		putter, err = s.newStamperPutter(context.Background(), putterOptions{
			BatchID:  headers.BatchID,
			TagID:    tag,
			Deferred: tag != 0,
		})
		if err != nil {
			logger.Debug("get putter failed", "error", err)
			logger.Error(nil, "get putter failed")
			switch {
			case errors.Is(err, errBatchUnusable) || errors.Is(err, postage.ErrNotUsable):
				jsonhttp.UnprocessableEntity(w, "batch not usable yet or does not exist")
			case errors.Is(err, postage.ErrNotFound):
				jsonhttp.NotFound(w, "batch with id not found")
			case errors.Is(err, errInvalidPostageBatch):
				jsonhttp.BadRequest(w, "invalid batch id")
			default:
				jsonhttp.BadRequest(w, nil)
			}
			return
		}
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  swarm.SocMaxChunkSize,
		WriteBufferSize: swarm.SocMaxChunkSize,
		CheckOrigin:     s.checkOrigin,
	}

	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Debug("chunk upload: upgrade failed", "error", err)
		logger.Error(nil, "chunk upload: upgrade failed")
		jsonhttp.BadRequest(w, "upgrade failed")
		return
	}

	s.wsWg.Add(1)
	var decode chunkDecoder
	if len(headers.BatchID) > 0 {
		decode = decodeChunkWithoutStamp
	} else {
		decode = decodeChunkWithStamp
	}
	go s.handleUploadStream(logger, wsConn, putter, tag, decode)
}

// chunkDecoder extracts chunk data and optionally a stamp from a websocket message.
// When BatchID is provided in headers, decodeChunkWithoutStamp is used (no stamp in message).
// When BatchID is not provided, decodeChunkWithStamp is used (stamp prepended to chunk data).
type chunkDecoder func(msg []byte) (chunkData []byte, stamp *postage.Stamp, err error)

// decodeChunkWithoutStamp returns the message as-is (used when BatchID provided in headers).
func decodeChunkWithoutStamp(msg []byte) ([]byte, *postage.Stamp, error) {
	return msg, nil, nil
}

// decodeChunkWithStamp extracts a stamp from the first 113 bytes of the message.
// Returns an error if the message is too small or the stamp is invalid.
func decodeChunkWithStamp(msg []byte) ([]byte, *postage.Stamp, error) {
	if len(msg) < postage.StampSize+swarm.SpanSize {
		return nil, nil, errors.New("message too small for stamp + chunk")
	}

	stamp := &postage.Stamp{}
	if err := stamp.UnmarshalBinary(msg[:postage.StampSize]); err != nil {
		return nil, nil, errors.New("invalid stamp")
	}

	return msg[postage.StampSize:], stamp, nil
}

func (s *Service) handleUploadStream(
	logger log.Logger,
	conn *websocket.Conn,
	putter storer.PutterSession,
	tag uint64,
	decode chunkDecoder,
) {
	defer s.wsWg.Done()

	ctx, cancel := context.WithCancel(context.Background())

	var (
		gone = make(chan struct{})
		err  error
	)

	// Cache for batch validation to avoid database lookups for every chunk
	// Key: batch ID, Value: stored batch info
	// This avoids the expensive batchStore.Get() call for each chunk
	batchCache := make(map[string]*postage.Batch)

	defer func() {
		cancel()
		_ = conn.Close()

		// No cleanup needed for batch cache - it's just metadata

		// Only call Done on connection-level putter if it exists
		if putter != nil {
			if err = putter.Done(swarm.ZeroAddress); err != nil {
				logger.Error(err, "chunk upload stream: syncing chunks failed")
			}
		}
	}()

	conn.SetCloseHandler(func(code int, text string) error {
		logger.Debug("chunk upload stream: client gone", "code", code, "message", text)
		close(gone)
		return nil
	})

	sendMsg := func(msgType int, buf []byte) error {
		err := conn.SetWriteDeadline(time.Now().Add(writeDeadline))
		if err != nil {
			return err
		}
		err = conn.WriteMessage(msgType, buf)
		if err != nil {
			return err
		}
		return nil
	}

	sendErrorClose := func(code int, errmsg string) {
		err := conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(code, errmsg),
			time.Now().Add(writeDeadline),
		)
		if err != nil {
			logger.Error(err, "chunk upload stream: failed sending close message")
		}
	}

	for {
		select {
		case <-s.quit:
			// shutdown
			sendErrorClose(websocket.CloseGoingAway, "node shutting down")
			return
		case <-gone:
			// client gone
			return
		default:
			// if there is no indication to stop, go ahead and read the next message
		}

		err = conn.SetReadDeadline(time.Now().Add(streamReadTimeout))
		if err != nil {
			logger.Debug("chunk upload stream: set read deadline failed", "error", err)
			logger.Error(nil, "chunk upload stream: set read deadline failed")
			return
		}

		mt, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Debug("chunk upload stream: read message failed", "error", err)
				logger.Error(nil, "chunk upload stream: read message failed")
			}
			return
		}

		if mt != websocket.BinaryMessage {
			logger.Debug("chunk upload stream: unexpected message received from client", "message_type", mt)
			logger.Error(nil, "chunk upload stream: unexpected message received from client")
			sendErrorClose(websocket.CloseUnsupportedData, "invalid message")
			return
		}

		if len(msg) < swarm.SpanSize {
			logger.Debug("chunk upload stream: insufficient data")
			logger.Error(nil, "chunk upload stream: insufficient data")
			return
		}

		// Decode the message using the appropriate decoder
		chunkData, stamp, err := decode(msg)
		if err != nil {
			logger.Debug("chunk upload stream: decode failed", "error", err)
			logger.Error(nil, "chunk upload stream: "+err.Error())
			sendErrorClose(websocket.CloseInternalServerErr, err.Error())
			return
		}

		// Determine the putter to use
		var (
			chunk       swarm.Chunk
			chunkPutter = putter
		)

		// If stamp was extracted, create a per-chunk putter
		if stamp != nil {
			batchID := stamp.BatchID()
			batchIDKey := string(batchID)

			storedBatch, exists := batchCache[batchIDKey]
			if !exists {
				storedBatch, err = s.batchStore.Get(batchID)
				if err != nil {
					logger.Debug("chunk upload stream: batch validation failed", "error", err)
					logger.Error(nil, "chunk upload stream: batch validation failed")
					if errors.Is(err, storage.ErrNotFound) {
						sendErrorClose(websocket.CloseInternalServerErr, "batch not found")
					} else {
						sendErrorClose(websocket.CloseInternalServerErr, "batch validation failed")
					}
					return
				}
				batchCache[batchIDKey] = storedBatch
			}

			chunkPutter, err = s.newStampedPutterWithBatch(ctx, putterOptions{
				BatchID:  batchID,
				TagID:    tag,
				Deferred: tag != 0,
			}, stamp, storedBatch)
			if err != nil {
				logger.Debug("chunk upload stream: failed to create stamped putter", "error", err)
				logger.Error(nil, "chunk upload stream: failed to create stamped putter")
				switch {
				case errors.Is(err, errBatchUnusable) || errors.Is(err, postage.ErrNotUsable):
					sendErrorClose(websocket.CloseInternalServerErr, "batch not usable")
				case errors.Is(err, postage.ErrNotFound):
					sendErrorClose(websocket.CloseInternalServerErr, "batch not found")
				default:
					sendErrorClose(websocket.CloseInternalServerErr, "stamped putter creation failed")
				}
				return
			}
		}

		chunk, err = cac.NewWithDataSpan(chunkData)
		if err != nil {
			logger.Debug("chunk upload stream: create chunk failed", "error", err, "chunk_size", len(chunkData))
			logger.Error(nil, "chunk upload stream: create chunk failed")
			if chunkPutter != putter {
				_ = chunkPutter.Cleanup()
			}
			sendErrorClose(websocket.CloseInternalServerErr, "invalid chunk data")
			return
		}

		err = chunkPutter.Put(ctx, chunk)
		if err != nil {
			logger.Debug("chunk upload stream: write chunk failed", "address", chunk.Address(), "error", err)
			logger.Error(nil, "chunk upload stream: write chunk failed")
			if chunkPutter != putter {
				_ = chunkPutter.Cleanup()
			}
			switch {
			case errors.Is(err, postage.ErrBucketFull):
				sendErrorClose(websocket.CloseInternalServerErr, "batch is overissued")
			default:
				sendErrorClose(websocket.CloseInternalServerErr, "chunk write error")
			}
			return
		}

		// Clean up per-chunk putter
		if chunkPutter != putter {
			if err := chunkPutter.Done(swarm.ZeroAddress); err != nil {
				logger.Error(err, "chunk upload stream: failed to finalize per-chunk putter")
			}
		}

		err = sendMsg(websocket.BinaryMessage, successWsMsg)
		if err != nil {
			s.logger.Debug("chunk upload stream: sending success message failed", "error", err)
			s.logger.Error(nil, "chunk upload stream: sending success message failed")
			return
		}
	}
}

func (s *Service) chunkBidirectionalStreamHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("chunks_stream_bidirectional").Build()

	headers := struct {
		// Optional: the batch the node stamps uploads with when a request names
		// neither a pre-signed stamp nor a batch of its own.
		BatchID []byte `map:"Swarm-Postage-Batch-Id"`
		Cache   *bool  `map:"Swarm-Cache"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	// Uploads on the stream are always direct, so a tag would have no effect.
	// Reject it rather than ignore it, so a client expecting deferred uploads
	// finds out at the handshake.
	if r.Header.Get(SwarmTagHeader) != "" || r.URL.Query().Get("swarm-tag") != "" {
		logger.Debug("chunk bidirectional stream: swarm-tag is not supported")
		jsonhttp.BadRequest(w, "swarm-tag is not supported on the chunk stream")
		return
	}

	stampers := newStreamStampers(s)
	if len(headers.BatchID) > 0 {
		// Resolve the connection default up front, so a bad batch fails the
		// handshake instead of every upload that relies on it.
		if _, _, err := stampers.get(headers.BatchID); err != nil {
			logger.Debug("get stamper failed", "error", err)
			switch {
			case errors.Is(err, errBatchUnusable) || errors.Is(err, postage.ErrNotUsable):
				jsonhttp.UnprocessableEntity(w, "batch not usable yet or does not exist")
			case errors.Is(err, postage.ErrNotFound):
				jsonhttp.NotFound(w, "batch with id not found")
			default:
				jsonhttp.BadRequest(w, "invalid batch id")
			}
			return
		}
	}

	defaultCache := true
	if qCache := r.URL.Query().Get("cache"); qCache != "" {
		c, err := strconv.ParseBool(qCache)
		if err != nil {
			logger.Debug("invalid cache query parameter", "value", qCache, "error", err)
			jsonhttp.BadRequest(w, "invalid cache query parameter")
			return
		}
		defaultCache = c
	}
	if headers.Cache != nil {
		defaultCache = *headers.Cache
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  swarm.SocMaxChunkSize,
		WriteBufferSize: swarm.SocMaxChunkSize,
		CheckOrigin:     s.checkOrigin,
		Subprotocols:    []string{chunkStreamSubprotocol},
	}

	s.wsWg.Add(1)
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.wsWg.Done()
		logger.Debug("chunk bidirectional stream: upgrade failed", "error", err)
		logger.Error(nil, "chunk bidirectional stream: upgrade failed")
		return
	}

	go s.handleBidirectionalStream(logger, wsConn, stampers, headers.BatchID, defaultCache)
}

// streamStampers resolves the node-side stamper for each postage batch used on
// one stream, so a batch is looked up once per connection rather than once per
// chunk. Failed lookups are not cached: a batch that is not usable yet may
// become usable while the connection is open.
type streamStampers struct {
	s       *Service
	mu      sync.Mutex
	byBatch map[string]streamStamper
}

type streamStamper struct {
	stamper postage.Stamper
	save    func() error
}

func newStreamStampers(s *Service) *streamStampers {
	return &streamStampers{s: s, byBatch: make(map[string]streamStamper)}
}

func (st *streamStampers) get(batchID []byte) (postage.Stamper, func() error, error) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if e, ok := st.byBatch[string(batchID)]; ok {
		return e.stamper, e.save, nil
	}
	stamper, save, err := st.s.getStamper(batchID)
	if err != nil {
		return nil, nil, err
	}
	st.byBatch[string(batchID)] = streamStamper{stamper: stamper, save: save}
	return stamper, save, nil
}

// getResponse builds a download reply. The address is echoed from the request,
// since it is the key the client correlates on.
func getResponse(address []byte, status pb.Status, data []byte, errMsg string) *pb.Response {
	return &pb.Response{Body: &pb.Response_Get{Get: &pb.GetResponse{
		Address: address,
		Status:  status,
		Data:    data,
		Error:   errMsg,
	}}}
}

// putResponse builds an upload reply. The address is echoed from the request,
// since it is the key the client correlates on.
func putResponse(address []byte, status pb.Status, errMsg string) *pb.Response {
	return &pb.Response{Body: &pb.Response_Put{Put: &pb.PutResponse{
		Address: address,
		Status:  status,
		Error:   errMsg,
	}}}
}

func (s *Service) handleBidirectionalStream(
	logger log.Logger,
	conn *websocket.Conn,
	stampers *streamStampers,
	defaultBatchID []byte,
	defaultCache bool,
) {
	defer s.wsWg.Done()

	s.metrics.ChunkStreamOpenConnections.WithLabelValues("stream").Inc()
	defer s.metrics.ChunkStreamOpenConnections.WithLabelValues("stream").Dec()

	ctx, cancel := context.WithCancel(context.Background())
	getQueue := make(chan *pb.GetRequest, maxStreamQueueSize)
	putQueue := make(chan *pb.PutRequest, maxStreamQueueSize)
	var workersWg sync.WaitGroup

	defer func() {
		cancel()
		// Connection must be closed BEFORE waiting on workers so stalled writes in conn.WriteMessage unblock.
		_ = conn.Close()
		close(getQueue)
		close(putQueue)
		workersWg.Wait()
	}()

	conn.SetReadLimit(maxStreamFrameSize)

	var writeMu sync.Mutex
	sendResponse := func(resp *pb.Response) error {
		data, err := resp.Marshal()
		if err != nil {
			return err
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		err = conn.SetWriteDeadline(time.Now().Add(s.chunkDeliveryWriteDeadline))
		if err != nil {
			cancel()
			return err
		}
		err = conn.WriteMessage(websocket.BinaryMessage, data)
		if err != nil {
			cancel()
			return err
		}
		return nil
	}

	sendErrorClose := func(code int, errmsg string) {
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(code, errmsg),
			time.Now().Add(chunkStreamCloseDeadline),
		)
	}

	gone := make(chan struct{})
	conn.SetCloseHandler(func(code int, text string) error {
		logger.Debug("chunk bidirectional stream: client gone", "code", code, "message", text)
		close(gone)
		return nil
	})

	go func() {
		select {
		case <-s.quit:
			cancel()
			sendErrorClose(websocket.CloseGoingAway, "node shutting down")
			_ = conn.Close()
		case <-ctx.Done():
		}
	}()

	batchCache := make(map[string]*postage.Batch)
	var batchCacheMu sync.RWMutex

	// Dedicated download workers (fairness: downloads never starve behind upload batches)
	for range defaultStreamSubWorkers {
		workersWg.Add(1)
		go func() {
			defer workersWg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case req, ok := <-getQueue:
					if !ok {
						return
					}
					s.processStreamGetRequest(ctx, logger, req, defaultCache, sendResponse)
				}
			}
		}()
	}

	// Dedicated upload workers
	for range defaultStreamSubWorkers {
		workersWg.Add(1)
		go func() {
			defer workersWg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case req, ok := <-putQueue:
					if !ok {
						return
					}
					s.processStreamPutRequest(ctx, logger, req, stampers, defaultBatchID, batchCache, &batchCacheMu, sendResponse)
				}
			}
		}()
	}

	for {
		select {
		case <-gone:
			return
		case <-ctx.Done():
			return
		default:
		}

		err := conn.SetReadDeadline(time.Now().Add(streamReadTimeout))
		if err != nil {
			logger.Debug("chunk bidirectional stream: set read deadline failed", "error", err)
			return
		}

		mt, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Debug("chunk bidirectional stream: read message failed", "error", err)
			}
			return
		}

		if mt != websocket.BinaryMessage {
			logger.Debug("chunk bidirectional stream: unexpected message type from client", "message_type", mt)
			sendErrorClose(websocket.CloseUnsupportedData, "invalid message type")
			return
		}

		req := &pb.Request{}
		if err := req.Unmarshal(msg); err != nil {
			logger.Debug("chunk bidirectional stream: unmarshal request failed", "error", err)
			sendErrorClose(websocket.CloseUnsupportedData, "invalid protobuf request")
			return
		}

		// A request is answered by echoing its address, so one without a body
		// has nothing to answer with and is treated like an undecodable message.
		switch body := req.GetBody().(type) {
		case *pb.Request_Get:
			if body.Get == nil {
				sendErrorClose(websocket.CloseUnsupportedData, "missing request body")
				return
			}
			select {
			case getQueue <- body.Get:
			case <-ctx.Done():
				return
			default:
				_ = sendResponse(getResponse(body.Get.Address, pb.Status_STATUS_BUSY, nil, "request queue full"))
			}
		case *pb.Request_Put:
			if body.Put == nil {
				sendErrorClose(websocket.CloseUnsupportedData, "missing request body")
				return
			}
			select {
			case putQueue <- body.Put:
			case <-ctx.Done():
				return
			default:
				_ = sendResponse(putResponse(body.Put.Address, pb.Status_STATUS_BUSY, "request queue full"))
			}
		default:
			logger.Debug("chunk bidirectional stream: request has no body")
			sendErrorClose(websocket.CloseUnsupportedData, "missing request body")
			return
		}
	}
}

func (s *Service) processStreamGetRequest(
	streamCtx context.Context,
	logger log.Logger,
	req *pb.GetRequest,
	defaultCache bool,
	sendResponse func(*pb.Response) error,
) {
	reply := func(resp *pb.Response) {
		if err := sendResponse(resp); err != nil {
			logger.Debug("chunk bidirectional stream: send get response failed", "address", req.Address, "error", err)
		}
	}

	if len(req.Address) != swarm.HashSize {
		reply(getResponse(req.Address, pb.Status_STATUS_BAD_REQUEST, nil, "invalid chunk address length"))
		return
	}

	addr := swarm.NewAddress(req.Address)
	cache := defaultCache
	switch req.Cache {
	case pb.CacheOption_CACHE_ENABLE:
		cache = true
	case pb.CacheOption_CACHE_DISABLE:
		cache = false
	}

	ctx, cancel := context.WithTimeout(streamCtx, s.chunkDownloadRequestTimeout)
	start := time.Now()
	chunk, err := s.storer.Download(cache).Get(ctx, addr)
	cancel()
	s.metrics.ChunkStreamFetchDuration.Observe(time.Since(start).Seconds())

	if err != nil {
		if streamCtx.Err() != nil {
			return
		}
		status := pb.Status_STATUS_ERROR
		errMsg := "chunk read error"
		if errors.Is(err, storage.ErrNotFound) || errors.Is(err, topology.ErrNotFound) {
			status = pb.Status_STATUS_NOT_FOUND
			errMsg = "chunk not found"
			s.metrics.ChunkStreamDeliveryCount.WithLabelValues("not_found").Inc()
			logger.V(1).Build().Debug("chunk bidirectional stream: chunk not found", "address", addr)
		} else if errors.Is(err, context.DeadlineExceeded) {
			errMsg = "chunk retrieval timed out"
			s.metrics.ChunkStreamDeliveryCount.WithLabelValues("error").Inc()
			logger.Debug("chunk bidirectional stream: chunk retrieval timed out", "address", addr, "timeout", s.chunkDownloadRequestTimeout)
		} else {
			s.metrics.ChunkStreamDeliveryCount.WithLabelValues("error").Inc()
			logger.Debug("chunk bidirectional stream: read chunk failed", "address", addr, "error", err)
		}
		reply(getResponse(req.Address, status, nil, errMsg))
		return
	}

	s.metrics.ChunkStreamDeliveryCount.WithLabelValues("success").Inc()
	reply(getResponse(req.Address, pb.Status_STATUS_OK, chunk.Data(), ""))
}

// chunkForAddress rebuilds an uploaded chunk and checks that it is the chunk the
// request names. The address also settles the chunk type: small SOC data parses
// as a valid CAC too, but only one reading of the data produces the address.
func chunkForAddress(address, data []byte) (swarm.Chunk, bool) {
	want := swarm.NewAddress(address)
	if ch, err := cac.NewWithDataSpan(data); err == nil && ch.Address().Equal(want) {
		return ch, true
	}
	sch, err := soc.FromChunk(swarm.NewChunk(swarm.EmptyAddress, data))
	if err != nil {
		return nil, false
	}
	ch, err := sch.Chunk()
	if err != nil || !soc.Valid(ch) || !ch.Address().Equal(want) {
		return nil, false
	}
	return ch, true
}

func (s *Service) processStreamPutRequest(
	streamCtx context.Context,
	logger log.Logger,
	req *pb.PutRequest,
	stampers *streamStampers,
	defaultBatchID []byte,
	batchCache map[string]*postage.Batch,
	batchCacheMu *sync.RWMutex,
	sendResponse func(*pb.Response) error,
) {
	reply := func(status pb.Status, errMsg string) {
		if status == pb.Status_STATUS_OK {
			s.metrics.ChunkStreamDeliveryCount.WithLabelValues("upload_success").Inc()
		} else {
			s.metrics.ChunkStreamDeliveryCount.WithLabelValues("upload_error").Inc()
		}
		if err := sendResponse(putResponse(req.Address, status, errMsg)); err != nil {
			logger.Debug("chunk bidirectional stream: send put response failed", "address", req.Address, "error", err)
		}
	}

	if len(req.Address) != swarm.HashSize {
		reply(pb.Status_STATUS_BAD_REQUEST, "invalid chunk address length")
		return
	}
	if len(req.Data) < swarm.SpanSize {
		reply(pb.Status_STATUS_BAD_REQUEST, "insufficient data for chunk")
		return
	}

	chunk, ok := chunkForAddress(req.Address, req.Data)
	if !ok {
		reply(pb.Status_STATUS_BAD_REQUEST, "address does not match chunk data")
		return
	}

	idAddr, err := storage.IdentityAddress(chunk)
	if err != nil {
		reply(pb.Status_STATUS_BAD_REQUEST, "cannot compute identity address")
		return
	}

	// Stamping, in order of precedence: a pre-signed stamp on the request, the
	// request's own batch, then the connection's batch.
	var stamp *postage.Stamp
	switch {
	case len(req.Stamp) > 0:
		presigned := &postage.Stamp{}
		if err := presigned.UnmarshalBinary(req.Stamp); err != nil {
			reply(pb.Status_STATUS_BAD_REQUEST, "invalid postage stamp")
			return
		}

		batchID := presigned.BatchID()
		batchIDKey := string(batchID)

		batchCacheMu.RLock()
		storedBatch, exists := batchCache[batchIDKey]
		batchCacheMu.RUnlock()

		if !exists {
			storedBatch, err = s.batchStore.Get(batchID)
			if err != nil {
				logger.Debug("chunk bidirectional stream: batch validation failed", "batch_id", batchID, "error", err)
				reply(pb.Status_STATUS_BAD_REQUEST, "postage batch not found or unusable")
				return
			}
			batchCacheMu.Lock()
			batchCache[batchIDKey] = storedBatch
			batchCacheMu.Unlock()
		}

		stamp, err = postage.NewPresignedStamper(presigned, storedBatch.Owner).Stamp(chunk.Address(), idAddr)
		if err != nil {
			reply(pb.Status_STATUS_BAD_REQUEST, "invalid postage stamp")
			return
		}
	default:
		batchID := req.BatchId
		if len(batchID) == 0 {
			batchID = defaultBatchID
		}
		if len(batchID) == 0 {
			reply(pb.Status_STATUS_BAD_REQUEST, "missing postage stamp or batch id")
			return
		}
		if len(batchID) != swarm.HashSize {
			reply(pb.Status_STATUS_BAD_REQUEST, "invalid batch id")
			return
		}

		stamper, save, err := stampers.get(batchID)
		if err != nil {
			logger.Debug("chunk bidirectional stream: get stamper failed", "batch_id", batchID, "error", err)
			switch {
			case errors.Is(err, errBatchUnusable) || errors.Is(err, postage.ErrNotUsable):
				reply(pb.Status_STATUS_BAD_REQUEST, "postage batch not usable")
			case errors.Is(err, postage.ErrNotFound):
				reply(pb.Status_STATUS_BAD_REQUEST, "postage batch not found")
			default:
				reply(pb.Status_STATUS_ERROR, "failed to stamp chunk")
			}
			return
		}

		stamp, err = stamper.Stamp(chunk.Address(), idAddr)
		if err != nil {
			logger.Debug("chunk bidirectional stream: stamp failed", "error", err)
			errMsg := "failed to stamp chunk"
			if errors.Is(err, postage.ErrBucketFull) {
				errMsg = "batch is overissued"
			}
			reply(pb.Status_STATUS_ERROR, errMsg)
			return
		}
		if err := save(); err != nil {
			logger.Debug("chunk bidirectional stream: save stamp state failed", "error", err)
		}
	}

	// Uploads are always direct: the reply waits for the push to complete.
	putCtx, cancel := context.WithTimeout(streamCtx, chunkUploadRequestTimeout)
	defer cancel()

	session := s.storer.DirectUpload()
	if err := session.Put(putCtx, chunk.WithStamp(stamp)); err != nil {
		if streamCtx.Err() != nil {
			return
		}
		logger.Debug("chunk bidirectional stream: direct upload put failed", "address", chunk.Address(), "error", err)
		reply(pb.Status_STATUS_ERROR, "chunk write error")
		return
	}
	if err := session.Done(swarm.ZeroAddress); err != nil {
		if streamCtx.Err() != nil {
			return
		}
		logger.Debug("chunk bidirectional stream: direct upload push failed", "address", chunk.Address(), "error", err)
		reply(pb.Status_STATUS_ERROR, "chunk push failed")
		return
	}

	reply(pb.Status_STATUS_OK, "")
}
