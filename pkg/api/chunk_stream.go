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

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy/getter"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/gorilla/websocket"
)

const (
	streamReadTimeout = 15 * time.Minute

	// chunkDeliveryWriteDeadline bounds a single delivery write. It is
	// deliberately generous: a client that buffers ahead — a media player
	// holding a lookahead window, say — stops reading from the socket while its
	// buffer drains, and must not have its whole stream torn down for it. This
	// is a backstop against a peer that has gone away, not flow control; how
	// much is in flight is governed by how much the client requests.
	chunkDeliveryWriteDeadline = 5 * time.Minute

	// chunkDownloadRequestTimeout bounds a single chunk retrieval. Without it a
	// worker can stay parked on one unreachable chunk while the rest of the
	// queue waits behind it. getter.DefaultFetchTimeout is what the joiner uses
	// for the same job.
	chunkDownloadRequestTimeout = getter.DefaultFetchTimeout

	// chunkStreamCloseDeadline bounds a control frame write. Control frames do
	// not inherit the delivery deadline: tearing the stream down must not wait
	// on a peer that has stopped reading.
	chunkStreamCloseDeadline = 5 * time.Second

	chunkDownloadSubprotocol = "swarm-chunk-download"
	chunkUploadSubprotocol   = "swarm-chunk-upload"

	// chunkDownloadOpcode is the command byte every download request frame
	// starts with. Framing requests as [opcode][32-byte address]... keeps room
	// for further commands to be added without breaking existing clients.
	chunkDownloadOpcode byte = 'D'

	wsChunkDeliverySuccess  byte = 0x00
	wsChunkDeliveryNotFound byte = 0x01
	wsChunkDeliveryError    byte = 0x02

	defaultDownloadWorkers = 16
	maxDownloadBatchSize   = 256

	// maxDownloadQueueSize is a multiple of maxDownloadBatchSize so that a
	// client can pipeline several maximal frames. With a queue only one batch
	// deep, a single full frame fills it and the read loop stops accepting
	// frames until the workers drain.
	maxDownloadQueueSize = 4 * maxDownloadBatchSize

	// maxDownloadFrameSize bounds a single inbound frame. It is deliberately
	// larger than maxDownloadBatchSize addresses so that a moderately
	// over-sized batch is rejected with an explicit close reason rather than
	// the bare transport-level 1009 that SetReadLimit produces.
	maxDownloadFrameSize = 2 * maxDownloadBatchSize * swarm.HashSize
)

var successWsMsg = []byte{}

func (s *Service) chunkStreamHandler(w http.ResponseWriter, r *http.Request) {
	// Reject plain HTTP requests before any tag, putter or header work is done.
	// Once upgrader.Upgrade is reached it writes its own error response, so
	// handlers below must not write another one.
	if !websocket.IsWebSocketUpgrade(r) {
		logger := s.logger.WithName("chunks_stream").Build()
		logger.Debug("chunk stream: not a websocket upgrade request")
		jsonhttp.BadRequest(w, "not a websocket upgrade request")
		return
	}

	// Reject an unknown mode rather than silently falling back to upload: the
	// client would otherwise only find out when its first address frame is
	// parsed as chunk data and the stream is torn down.
	mode := r.URL.Query().Get("mode")
	switch mode {
	case "", "upload", "download":
	default:
		logger := s.logger.WithName("chunks_stream").Build()
		logger.Debug("chunk stream: invalid mode query parameter", "value", mode)
		jsonhttp.BadRequest(w, "invalid mode query parameter")
		return
	}

	// The download subprotocol takes precedence over the mode query parameter.
	if slices.Contains(websocket.Subprotocols(r), chunkDownloadSubprotocol) || mode == "download" {
		s.chunkDownloadStreamHandler(w, r)
		return
	}
	s.chunkUploadStreamHandler(w, r)
}

func (s *Service) chunkUploadStreamHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("chunks_stream_upload").Build()

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
		Subprotocols:    []string{chunkUploadSubprotocol},
	}

	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Debug("chunk upload: upgrade failed", "error", err)
		logger.Error(nil, "chunk upload: upgrade failed")
		// Upgrade writes its own error response; the putter is owned by this
		// function until handleUploadStream takes it over, so release it here.
		if putter != nil {
			if err := putter.Cleanup(); err != nil {
				logger.Debug("chunk upload: putter cleanup failed", "error", err)
			}
		}
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

func (s *Service) chunkDownloadStreamHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("chunks_stream_download").Build()

	headers := struct {
		Cache *bool `map:"Swarm-Cache"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	cache := true
	if qCache := r.URL.Query().Get("cache"); qCache != "" {
		c, err := strconv.ParseBool(qCache)
		if err != nil {
			logger.Debug("invalid cache query parameter", "value", qCache, "error", err)
			jsonhttp.BadRequest(w, "invalid cache query parameter")
			return
		}
		cache = c
	}
	// The Swarm-Cache header takes precedence over the query parameter, which
	// exists for clients that cannot set custom headers.
	if headers.Cache != nil {
		cache = *headers.Cache
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  swarm.SocMaxChunkSize,
		WriteBufferSize: swarm.SocMaxChunkSize,
		CheckOrigin:     s.checkOrigin,
		Subprotocols:    []string{chunkDownloadSubprotocol},
	}

	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Debug("chunk download: upgrade failed", "error", err)
		logger.Error(nil, "chunk download: upgrade failed")
		return
	}

	s.wsWg.Add(1)
	go s.handleDownloadStream(logger, wsConn, cache)
}

func (s *Service) handleDownloadStream(
	logger log.Logger,
	conn *websocket.Conn,
	cache bool,
) {
	defer s.wsWg.Done()

	s.metrics.ChunkStreamOpenConnections.WithLabelValues("download").Inc()
	defer s.metrics.ChunkStreamOpenConnections.WithLabelValues("download").Dec()

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		_ = conn.Close()
	}()

	conn.SetReadLimit(maxDownloadFrameSize)

	var writeMu sync.Mutex
	sendMsg := func(data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		err := conn.SetWriteDeadline(time.Now().Add(s.chunkDeliveryWriteDeadline))
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

	// WriteControl may be called concurrently with WriteMessage, so this
	// deliberately does not take writeMu: a delivery that is blocked on a slow
	// reader must not delay the close frame that tells the client why.
	sendErrorClose := func(code int, errmsg string) {
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(code, errmsg),
			time.Now().Add(chunkStreamCloseDeadline),
		)
	}

	gone := make(chan struct{})
	conn.SetCloseHandler(func(code int, text string) error {
		logger.Debug("chunk download stream: client gone", "code", code, "message", text)
		close(gone)
		return nil
	})

	// The read loop can sit in ReadMessage for streamReadTimeout, so it cannot
	// notice s.quit on its own. Closing the connection makes the pending read
	// return at once, which is what lets api.Close finish inside its budget.
	go func() {
		select {
		case <-s.quit:
			sendErrorClose(websocket.CloseGoingAway, "node shutting down")
			_ = conn.Close()
		case <-ctx.Done():
		}
	}()

	loggerV1 := logger.V(1).Build()
	jobs := make(chan swarm.Address, maxDownloadQueueSize)
	var wg sync.WaitGroup

	for range defaultDownloadWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case addr, ok := <-jobs:
					if !ok {
						return
					}
					s.fetchAndSendChunk(ctx, logger, loggerV1, addr, cache, sendMsg)
				}
			}
		}()
	}

	defer func() {
		cancel()
		close(jobs)
		wg.Wait()
	}()

	for {
		select {
		case <-s.quit:
			// The watcher goroutine above sends the close frame.
			return
		case <-gone:
			return
		default:
		}

		err := conn.SetReadDeadline(time.Now().Add(streamReadTimeout))
		if err != nil {
			logger.Debug("chunk download stream: set read deadline failed", "error", err)
			return
		}

		mt, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Debug("chunk download stream: read message failed", "error", err)
			}
			return
		}

		if mt != websocket.BinaryMessage {
			logger.Debug("chunk download stream: unexpected message received from client", "message_type", mt)
			sendErrorClose(websocket.CloseUnsupportedData, "invalid message")
			return
		}

		if len(msg) < 1+swarm.HashSize || (len(msg)-1)%swarm.HashSize != 0 {
			logger.Debug("chunk download stream: invalid message length", "length", len(msg))
			sendErrorClose(websocket.CloseUnsupportedData, "invalid message length")
			return
		}

		if msg[0] != chunkDownloadOpcode {
			logger.Debug("chunk download stream: unknown command", "opcode", msg[0])
			sendErrorClose(websocket.CloseUnsupportedData, "unknown command")
			return
		}

		payload := msg[1:]
		batchCount := len(payload) / swarm.HashSize
		if batchCount > maxDownloadBatchSize {
			logger.Debug("chunk download stream: batch size exceeds limit", "count", batchCount)
			sendErrorClose(websocket.CloseMessageTooBig, "batch size exceeds limit")
			return
		}

		// swarm.NewAddress does not copy: every address below aliases msg, which
		// stays alive until the last worker is done with it. This is safe only
		// because ReadMessage allocates a fresh buffer per message; a pooled or
		// reused read buffer would corrupt addresses across concurrent workers.
		addrs := make([]swarm.Address, 0, batchCount)
		for i := 0; i < len(payload); i += swarm.HashSize {
			addrs = append(addrs, swarm.NewAddress(payload[i:i+swarm.HashSize]))
		}

		for _, addr := range addrs {
			select {
			case jobs <- addr:
			case <-ctx.Done():
				return
			case <-s.quit:
				return
			}
		}
	}
}

// fetchAndSendChunk retrieves a single chunk and writes exactly one response
// frame for it. That one-frame-per-requested-address invariant is what lets a
// client account for every address it asked for: responses carry no request id,
// so a dropped frame is indistinguishable from a slow one. The only exception
// is a stream that is already going away, where nobody is left to read.
func (s *Service) fetchAndSendChunk(
	streamCtx context.Context,
	logger log.Logger,
	loggerV1 log.Logger,
	addr swarm.Address,
	cache bool,
	sendMsg func([]byte) error,
) {
	// The per-request timeout is derived from, but distinct from, the stream
	// context: a request that times out still owes the client a status frame,
	// and only a stream that is going away may be answered with silence.
	ctx, cancel := context.WithTimeout(streamCtx, s.chunkDownloadRequestTimeout)
	start := time.Now()
	chunk, err := s.storer.Download(cache).Get(ctx, addr)
	cancel()
	s.metrics.ChunkStreamFetchDuration.Observe(time.Since(start).Seconds())

	if err != nil {
		if streamCtx.Err() != nil {
			return
		}
		status := wsChunkDeliveryError
		// topology.ErrNotFound means no peer could serve the chunk, which the
		// HTTP download path also reports as not found (see bzz.go).
		if errors.Is(err, storage.ErrNotFound) || errors.Is(err, topology.ErrNotFound) {
			status = wsChunkDeliveryNotFound
			loggerV1.Debug("chunk download stream: chunk not found", "address", addr)
		} else if errors.Is(err, context.DeadlineExceeded) {
			logger.Debug("chunk download stream: chunk retrieval timed out", "address", addr, "timeout", s.chunkDownloadRequestTimeout)
		} else {
			logger.Debug("chunk download stream: read chunk failed", "address", addr, "error", err)
		}
		if status == wsChunkDeliveryNotFound {
			s.metrics.ChunkStreamDeliveryCount.WithLabelValues("not_found").Inc()
		} else {
			s.metrics.ChunkStreamDeliveryCount.WithLabelValues("error").Inc()
		}

		resp := make([]byte, 1+swarm.HashSize)
		resp[0] = status
		copy(resp[1:], addr.Bytes())
		if err := sendMsg(resp); err != nil {
			logger.Debug("chunk download stream: send status message failed", "address", addr, "error", err)
		}
		return
	}

	s.metrics.ChunkStreamDeliveryCount.WithLabelValues("success").Inc()

	chunkData := chunk.Data()
	resp := make([]byte, 1+swarm.HashSize+len(chunkData))
	resp[0] = wsChunkDeliverySuccess
	copy(resp[1:1+swarm.HashSize], addr.Bytes())
	copy(resp[1+swarm.HashSize:], chunkData)

	if err := sendMsg(resp); err != nil {
		logger.Debug("chunk download stream: send chunk message failed", "address", addr, "error", err)
	}
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
