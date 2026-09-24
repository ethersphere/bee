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
		BatchID  []byte `map:"Swarm-Postage-Batch-Id"` // Optional
		SwarmTag uint64 `map:"Swarm-Tag"`
		Cache    *bool  `map:"Swarm-Cache"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

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

	var (
		connStamper    postage.Stamper
		connStampSave  func() error
		deferredPutter storer.PutterSession
	)

	if len(headers.BatchID) > 0 {
		stamper, save, err := s.getStamper(headers.BatchID)
		if err != nil {
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
		connStamper = stamper
		connStampSave = save
	}

	// If a tag was specified, set up a deferred putter session for the connection
	if tag > 0 {
		deferredPutter, err = s.storer.Upload(context.Background(), false, tag)
		if err != nil {
			logger.Debug("create deferred putter failed", "error", err)
			jsonhttp.InternalServerError(w, "cannot create upload session")
			return
		}
	}

	defaultCache := true
	if qCache := r.URL.Query().Get("cache"); qCache != "" {
		c, err := strconv.ParseBool(qCache)
		if err != nil {
			logger.Debug("invalid cache query parameter", "value", qCache, "error", err)
			if deferredPutter != nil {
				_ = deferredPutter.Cleanup()
			}
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
		if deferredPutter != nil {
			if err := deferredPutter.Cleanup(); err != nil {
				logger.Debug("chunk bidirectional stream: deferred putter cleanup failed", "error", err)
			}
		}
		return
	}

	go s.handleBidirectionalStream(logger, wsConn, connStamper, connStampSave, deferredPutter, tag, defaultCache)
}

func (s *Service) handleBidirectionalStream(
	logger log.Logger,
	conn *websocket.Conn,
	connStamper postage.Stamper,
	connStampSave func() error,
	deferredPutter storer.PutterSession,
	tag uint64,
	defaultCache bool,
) {
	defer s.wsWg.Done()

	s.metrics.ChunkStreamOpenConnections.WithLabelValues("stream").Inc()
	defer s.metrics.ChunkStreamOpenConnections.WithLabelValues("stream").Dec()

	ctx, cancel := context.WithCancel(context.Background())
	getQueue := make(chan *pb.Request, maxStreamQueueSize)
	putQueue := make(chan *pb.Request, maxStreamQueueSize)
	var workersWg sync.WaitGroup

	defer func() {
		cancel()
		// Connection must be closed BEFORE waiting on workers so stalled writes in conn.WriteMessage unblock.
		_ = conn.Close()
		close(getQueue)
		close(putQueue)
		workersWg.Wait()

		if deferredPutter != nil {
			if err := deferredPutter.Done(swarm.ZeroAddress); err != nil {
				logger.Debug("chunk bidirectional stream: deferred putter done failed", "error", err)
			}
		}
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
					s.processStreamGetRequest(ctx, logger, req.Id, req.GetGet(), defaultCache, sendResponse)
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
					s.processStreamPutRequest(ctx, logger, req.Id, req.GetPut(), connStamper, connStampSave, deferredPutter, tag, batchCache, &batchCacheMu, sendResponse)
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

		switch req.GetBody().(type) {
		case *pb.Request_Get:
			select {
			case getQueue <- req:
			case <-ctx.Done():
				return
			default:
				resp := &pb.Response{
					Id:     req.Id,
					Status: pb.Status_STATUS_BUSY,
					Error:  "request queue full",
				}
				_ = sendResponse(resp)
			}
		case *pb.Request_Put:
			select {
			case putQueue <- req:
			case <-ctx.Done():
				return
			default:
				resp := &pb.Response{
					Id:     req.Id,
					Status: pb.Status_STATUS_BUSY,
					Error:  "request queue full",
				}
				_ = sendResponse(resp)
			}
		default:
			resp := &pb.Response{
				Id:     req.Id,
				Status: pb.Status_STATUS_BAD_REQUEST,
				Error:  "missing or invalid request body",
			}
			if err := sendResponse(resp); err != nil {
				logger.Debug("chunk bidirectional stream: send invalid request response failed", "id", req.Id, "error", err)
			}
		}
	}
}

func (s *Service) processStreamGetRequest(
	streamCtx context.Context,
	logger log.Logger,
	id uint64,
	getReq *pb.GetRequest,
	defaultCache bool,
	sendResponse func(*pb.Response) error,
) {
	if getReq == nil || len(getReq.Address) != swarm.HashSize {
		resp := &pb.Response{
			Id:     id,
			Status: pb.Status_STATUS_BAD_REQUEST,
			Error:  "invalid chunk address length",
		}
		if err := sendResponse(resp); err != nil {
			logger.Debug("chunk bidirectional stream: send get response failed", "id", id, "error", err)
		}
		return
	}

	addr := swarm.NewAddress(getReq.Address)
	cache := defaultCache
	switch getReq.Cache {
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

		resp := &pb.Response{
			Id:      id,
			Status:  status,
			Address: addr.Bytes(),
			Error:   errMsg,
		}
		if err := sendResponse(resp); err != nil {
			logger.Debug("chunk bidirectional stream: send get response failed", "id", id, "error", err)
		}
		return
	}

	s.metrics.ChunkStreamDeliveryCount.WithLabelValues("success").Inc()

	resp := &pb.Response{
		Id:      id,
		Status:  pb.Status_STATUS_OK,
		Address: addr.Bytes(),
		Data:    chunk.Data(),
	}
	if err := sendResponse(resp); err != nil {
		logger.Debug("chunk bidirectional stream: send get response failed", "id", id, "error", err)
	}
}

func (s *Service) processStreamPutRequest(
	streamCtx context.Context,
	logger log.Logger,
	id uint64,
	putReq *pb.PutRequest,
	connStamper postage.Stamper,
	connStampSave func() error,
	deferredPutter storer.PutterSession,
	tag uint64,
	batchCache map[string]*postage.Batch,
	batchCacheMu *sync.RWMutex,
	sendResponse func(*pb.Response) error,
) {
	reply := func(resp *pb.Response) error {
		if resp.Status == pb.Status_STATUS_OK {
			s.metrics.ChunkStreamDeliveryCount.WithLabelValues("upload_success").Inc()
		} else {
			s.metrics.ChunkStreamDeliveryCount.WithLabelValues("upload_error").Inc()
		}
		return sendResponse(resp)
	}

	if putReq == nil || len(putReq.Data) < swarm.SpanSize {
		resp := &pb.Response{
			Id:     id,
			Status: pb.Status_STATUS_BAD_REQUEST,
			Error:  "insufficient data for chunk",
		}
		_ = reply(resp)
		return
	}

	var chunk swarm.Chunk
	switch putReq.Type {
	case pb.ChunkType_CHUNK_TYPE_SOC:
		sch, err := soc.FromChunk(swarm.NewChunk(swarm.EmptyAddress, putReq.Data))
		if err != nil {
			resp := &pb.Response{
				Id:     id,
				Status: pb.Status_STATUS_BAD_REQUEST,
				Error:  "invalid soc chunk data",
			}
			_ = reply(resp)
			return
		}
		chunk, err = sch.Chunk()
		if err != nil || !soc.Valid(chunk) {
			resp := &pb.Response{
				Id:     id,
				Status: pb.Status_STATUS_BAD_REQUEST,
				Error:  "invalid soc chunk",
			}
			_ = reply(resp)
			return
		}
	case pb.ChunkType_CHUNK_TYPE_CAC:
		var err error
		chunk, err = cac.NewWithDataSpan(putReq.Data)
		if err != nil {
			resp := &pb.Response{
				Id:     id,
				Status: pb.Status_STATUS_BAD_REQUEST,
				Error:  "invalid chunk data",
			}
			_ = reply(resp)
			return
		}
	default:
		resp := &pb.Response{
			Id:     id,
			Status: pb.Status_STATUS_BAD_REQUEST,
			Error:  "unspecified or invalid chunk type",
		}
		_ = reply(resp)
		return
	}

	var stampedChunk swarm.Chunk

	if len(putReq.Stamp) > 0 {
		stamp := &postage.Stamp{}
		if err := stamp.UnmarshalBinary(putReq.Stamp); err != nil {
			resp := &pb.Response{
				Id:     id,
				Status: pb.Status_STATUS_BAD_REQUEST,
				Error:  "invalid postage stamp",
			}
			_ = reply(resp)
			return
		}

		batchID := stamp.BatchID()
		batchIDKey := string(batchID)

		batchCacheMu.RLock()
		storedBatch, exists := batchCache[batchIDKey]
		batchCacheMu.RUnlock()

		if !exists {
			var err error
			storedBatch, err = s.batchStore.Get(batchID)
			if err != nil {
				logger.Debug("chunk bidirectional stream: batch validation failed", "batch_id", batchID, "error", err)
				resp := &pb.Response{
					Id:     id,
					Status: pb.Status_STATUS_BAD_REQUEST,
					Error:  "postage batch not found or unusable",
				}
				_ = reply(resp)
				return
			}
			batchCacheMu.Lock()
			batchCache[batchIDKey] = storedBatch
			batchCacheMu.Unlock()
		}

		stamper := postage.NewPresignedStamper(stamp, storedBatch.Owner)
		idAddr, err := storage.IdentityAddress(chunk)
		if err != nil {
			resp := &pb.Response{
				Id:     id,
				Status: pb.Status_STATUS_BAD_REQUEST,
				Error:  "cannot compute identity address",
			}
			_ = reply(resp)
			return
		}
		stamp, err = stamper.Stamp(chunk.Address(), idAddr)
		if err != nil {
			resp := &pb.Response{
				Id:     id,
				Status: pb.Status_STATUS_BAD_REQUEST,
				Error:  "invalid postage stamp",
			}
			_ = reply(resp)
			return
		}
		stampedChunk = chunk.WithStamp(stamp)
	} else if connStamper != nil {
		idAddr, err := storage.IdentityAddress(chunk)
		if err != nil {
			resp := &pb.Response{
				Id:     id,
				Status: pb.Status_STATUS_BAD_REQUEST,
				Error:  "cannot compute identity address",
			}
			_ = reply(resp)
			return
		}
		stamp, err := connStamper.Stamp(chunk.Address(), idAddr)
		if err != nil {
			logger.Debug("chunk bidirectional stream: stamp failed", "error", err)
			errMsg := "failed to stamp chunk"
			if errors.Is(err, postage.ErrBucketFull) {
				errMsg = "batch is overissued"
			}
			resp := &pb.Response{
				Id:     id,
				Status: pb.Status_STATUS_ERROR,
				Error:  errMsg,
			}
			_ = reply(resp)
			return
		}
		if connStampSave != nil {
			if err := connStampSave(); err != nil {
				logger.Debug("chunk bidirectional stream: save stamp state failed", "error", err)
			}
		}
		stampedChunk = chunk.WithStamp(stamp)
	} else {
		resp := &pb.Response{
			Id:     id,
			Status: pb.Status_STATUS_BAD_REQUEST,
			Error:  "missing postage stamp and no batch ID specified for stream",
		}
		_ = reply(resp)
		return
	}

	putCtx, cancel := context.WithTimeout(streamCtx, chunkUploadRequestTimeout)
	defer cancel()

	if deferredPutter != nil {
		// Tagged deferred upload: writes directly to local storage
		err := deferredPutter.Put(putCtx, stampedChunk)
		if err != nil {
			if streamCtx.Err() != nil {
				return
			}
			logger.Debug("chunk bidirectional stream: deferred write chunk failed", "address", chunk.Address(), "error", err)
			resp := &pb.Response{
				Id:      id,
				Status:  pb.Status_STATUS_ERROR,
				Address: chunk.Address().Bytes(),
				Error:   "chunk write error",
			}
			_ = reply(resp)
			return
		}
	} else {
		// Direct upload: chunk is pushed to network peers, and we await push confirmation
		session := s.storer.DirectUpload()
		err := session.Put(putCtx, stampedChunk)
		if err != nil {
			if streamCtx.Err() != nil {
				return
			}
			logger.Debug("chunk bidirectional stream: direct upload put failed", "address", chunk.Address(), "error", err)
			resp := &pb.Response{
				Id:      id,
				Status:  pb.Status_STATUS_ERROR,
				Address: chunk.Address().Bytes(),
				Error:   "chunk write error",
			}
			_ = reply(resp)
			return
		}

		err = session.Done(swarm.ZeroAddress)
		if err != nil {
			if streamCtx.Err() != nil {
				return
			}
			logger.Debug("chunk bidirectional stream: direct upload push failed", "address", chunk.Address(), "error", err)
			resp := &pb.Response{
				Id:      id,
				Status:  pb.Status_STATUS_ERROR,
				Address: chunk.Address().Bytes(),
				Error:   "chunk push failed",
			}
			_ = reply(resp)
			return
		}
	}

	resp := &pb.Response{
		Id:      id,
		Status:  pb.Status_STATUS_OK,
		Address: chunk.Address().Bytes(),
	}
	if err := reply(resp); err != nil {
		logger.Debug("chunk bidirectional stream: send put response failed", "id", id, "error", err)
	}
}
