// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
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
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/gorilla/websocket"
	"golang.org/x/sync/errgroup"
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

	// chunkStreamCloseDeadline bounds a close frame write. It is kept short
	// because gorilla queues a control frame behind an in-progress write, and
	// the goroutine sending it also watches for node shutdown.
	chunkStreamCloseDeadline = 250 * time.Millisecond

	defaultStreamSubWorkers = 8
	maxStreamQueueSize      = 1024

	// maxStreamFrameSize bounds a client message, and with it the memory a full
	// request queue can hold. The largest valid request, a SOC put with a stamp
	// and a batch id, is under 4.5 KiB.
	maxStreamFrameSize = 8 * 1024

	// chunkStreamSubprotocol names this protocol and its version in the
	// websocket handshake. The upload-only stream it replaced was served on the
	// same path and selects no subprotocol, so a client that offers this one
	// and finds it not selected is talking to an older node and must not send
	// requests: that node would push them as chunks. Change it when the wire
	// format changes incompatibly.
	chunkStreamSubprotocol = "bee-chunk-stream-v1"
)

var errStreamShutdown = errors.New("node shutting down")

func (s *Service) chunkStreamHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("chunks_stream").Build()

	// Reject plain HTTP requests before any header or batch work is done.
	if !websocket.IsWebSocketUpgrade(r) {
		logger.Debug("chunk stream: not a websocket upgrade request")
		jsonhttp.BadRequest(w, "not a websocket upgrade request")
		return
	}

	headers := struct {
		// Optional: the batch the node stamps uploads with when a request names
		// neither a pre-signed stamp nor a batch of its own.
		BatchID []byte `map:"Swarm-Postage-Batch-Id"`
		Cache   *bool  `map:"Swarm-Cache"`
		// Uploads on the stream are always direct, so a tag would have no
		// effect. It is rejected rather than ignored, so a client expecting
		// deferred uploads finds out at the handshake.
		Tag *uint64 `map:"Swarm-Tag"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	if headers.Tag != nil || r.URL.Query().Get("swarm-tag") != "" {
		logger.Debug("chunk stream: swarm-tag is not supported")
		jsonhttp.BadRequest(w, "swarm-tag is not supported on the chunk stream")
		return
	}

	if len(headers.BatchID) > 0 {
		// Check the connection batch up front, so a bad batch fails the
		// handshake instead of every upload that relies on it.
		if _, _, err := s.getStamper(headers.BatchID); err != nil {
			logger.Debug("get stamper failed", "error", err)
			switch {
			case errors.Is(err, errBatchUnusable) || errors.Is(err, postage.ErrNotUsable):
				jsonhttp.UnprocessableEntity(w, "batch not usable yet or does not exist")
			case errors.Is(err, postage.ErrNotFound):
				jsonhttp.NotFound(w, "batch with id not found")
			default:
				// The batch store or the stamp issuer lookup failed.
				jsonhttp.InternalServerError(w, "cannot get batch")
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
		// Selected only when the client offers it, so clients that do not
		// still connect.
		Subprotocols: []string{chunkStreamSubprotocol},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Debug("chunk stream: upgrade failed", "error", err)
		logger.Error(nil, "chunk stream: upgrade failed")
		return
	}

	cs := &chunkStream{
		s:            s,
		logger:       logger,
		conn:         conn,
		defaultBatch: headers.BatchID,
		defaultCache: defaultCache,
		getQueue:     make(chan *pb.GetRequest, maxStreamQueueSize),
		putQueue:     make(chan *pb.PutRequest, maxStreamQueueSize),
	}
	s.wsWg.Add(1)
	go cs.run()
}

// chunkStream serves one /chunks/stream connection. A reader queues requests,
// separate worker pools answer downloads and uploads, so that slow uploads do
// not hold up downloads, and every reply is a write under writeMu.
type chunkStream struct {
	s            *Service
	logger       log.Logger
	conn         *websocket.Conn
	defaultBatch []byte
	defaultCache bool
	getQueue     chan *pb.GetRequest
	putQueue     chan *pb.PutRequest
	writeMu      sync.Mutex
}

// run serves the stream until it ends. The first error ends it: a reader
// error, including a clean close by the client, a failed reply write, or node
// shutdown. A request that fails is answered and does not end the stream.
func (cs *chunkStream) run() {
	defer cs.s.wsWg.Done()

	cs.s.metrics.ChunkStreamOpenConnections.Inc()
	defer cs.s.metrics.ChunkStreamOpenConnections.Dec()

	cs.conn.SetReadLimit(maxStreamFrameSize)
	cs.setKeepaliveHandlers()

	eg, ctx := errgroup.WithContext(context.Background())
	eg.Go(func() error { return cs.watch(ctx) })
	eg.Go(func() error { return cs.ping(ctx) })
	for range defaultStreamSubWorkers {
		eg.Go(func() error { return serveQueue(ctx, cs.getQueue, cs.handleGet) })
		eg.Go(func() error { return serveQueue(ctx, cs.putQueue, cs.handlePut) })
	}
	eg.Go(func() error { return cs.read(ctx) })

	if err := eg.Wait(); err != nil {
		cs.logger.Debug("chunk stream: ended", "error", err)
	}
}

// serveQueue answers requests from queue until ctx ends. Queued requests are
// left behind when the stream ends.
func serveQueue[T any](ctx context.Context, queue <-chan T, handle func(context.Context, T) error) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case req := <-queue:
			if err := handle(ctx, req); err != nil {
				return err
			}
		}
	}
}

// watch closes the socket when the stream ends, which is the only thing that
// releases a blocked read or write, and sends a close frame first on node
// shutdown.
func (cs *chunkStream) watch(ctx context.Context) error {
	var err error
	select {
	case <-cs.s.quit:
		cs.sendClose(websocket.CloseGoingAway, "node shutting down")
		err = errStreamShutdown
	case <-ctx.Done():
	}
	_ = cs.conn.Close()
	return err
}

// ping keeps a client that is only waiting for replies within the read
// deadline: its pongs extend it.
func (cs *chunkStream) ping(ctx context.Context) error {
	ticker := time.NewTicker(cs.s.WsPingPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			err := cs.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(cs.s.chunkDeliveryWriteDeadline))
			if keepaliveWriteFailed(err) {
				return fmt.Errorf("write ping: %w", err)
			}
		}
	}
}

// keepaliveWriteFailed reports whether a keepalive write error ends the stream.
// A keepalive can wait for as long as a reply, behind replies to a client that
// has paused reading, so a timeout does not end it. Neither does a write after
// the close frame: the stream is already ending.
func keepaliveWriteFailed(err error) bool {
	if err == nil || errors.Is(err, websocket.ErrCloseSent) {
		return false
	}
	var netErr net.Error
	return !errors.As(err, &netErr) || !netErr.Timeout()
}

// setKeepaliveHandlers makes pongs and client pings extend the read deadline,
// as requests do. gorilla treats a control frame write that times out as fatal
// to the connection, so a pong gets the same deadline as a reply: a client
// that pauses reading must not lose its stream to a keepalive.
func (cs *chunkStream) setKeepaliveHandlers() {
	extend := func() error {
		return cs.conn.SetReadDeadline(time.Now().Add(cs.s.chunkStreamReadTimeout))
	}
	cs.conn.SetPongHandler(func(string) error { return extend() })
	cs.conn.SetPingHandler(func(data string) error {
		_ = extend()
		err := cs.conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(cs.s.chunkDeliveryWriteDeadline))
		if keepaliveWriteFailed(err) {
			return fmt.Errorf("write pong: %w", err)
		}
		return nil
	})
}

// read queues requests until the stream ends. It always returns an error, so
// that a client closing the stream ends the group.
func (cs *chunkStream) read(ctx context.Context) error {
	for {
		if err := cs.conn.SetReadDeadline(time.Now().Add(cs.s.chunkStreamReadTimeout)); err != nil {
			return fmt.Errorf("set read deadline: %w", err)
		}
		mt, msg, err := cs.conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read message: %w", err)
		}
		if mt != websocket.BinaryMessage {
			cs.sendClose(websocket.CloseUnsupportedData, "invalid message type")
			return fmt.Errorf("unexpected message type %d", mt)
		}

		req := &pb.Request{}
		if err := req.Unmarshal(msg); err != nil {
			cs.sendClose(websocket.CloseUnsupportedData, "invalid protobuf request")
			return fmt.Errorf("unmarshal request: %w", err)
		}

		// A request is answered by echoing its address, so one without a body
		// has nothing to answer with and is treated like an undecodable message.
		switch body := req.GetBody().(type) {
		case *pb.Request_Get:
			err = enqueue(ctx, cs, cs.getQueue, body.Get, getResponse(body.Get.Address, pb.Status_STATUS_BUSY, nil, "request queue full"))
		case *pb.Request_Put:
			err = enqueue(ctx, cs, cs.putQueue, body.Put, putResponse(body.Put.Address, pb.Status_STATUS_BUSY, "request queue full"))
		default:
			cs.sendClose(websocket.CloseUnsupportedData, "missing request body")
			return errors.New("request has no body")
		}
		if err != nil {
			return fmt.Errorf("enqueue request: %w", err)
		}
	}
}

// enqueue queues req, or answers it with busy when the queue is full. The busy
// reply waits for the socket like any other, so a client that stops reading
// replies eventually stops the node reading its requests.
func enqueue[T any](ctx context.Context, cs *chunkStream, queue chan<- T, req T, busy *pb.Response) error {
	select {
	case queue <- req:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		cs.s.metrics.ChunkStreamDeliveryCount.WithLabelValues("busy").Inc()
		return cs.send(busy)
	}
}

// send writes a reply. An error means the stream can no longer be written to.
func (cs *chunkStream) send(resp *pb.Response) error {
	data, err := resp.Marshal()
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}
	cs.writeMu.Lock()
	defer cs.writeMu.Unlock()
	if err := cs.conn.SetWriteDeadline(time.Now().Add(cs.s.chunkDeliveryWriteDeadline)); err != nil {
		return fmt.Errorf("set write deadline: %w", err)
	}
	if err := cs.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return fmt.Errorf("write response: %w", err)
	}
	return nil
}

func (cs *chunkStream) sendClose(code int, text string) {
	_ = cs.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, text), time.Now().Add(chunkStreamCloseDeadline))
}

func getResponse(address []byte, status pb.Status, data []byte, errMsg string) *pb.Response {
	return &pb.Response{Body: &pb.Response_Get{Get: &pb.GetResponse{
		Address: address,
		Status:  status,
		Data:    data,
		Error:   errMsg,
	}}}
}

func putResponse(address []byte, status pb.Status, errMsg string) *pb.Response {
	return &pb.Response{Body: &pb.Response_Put{Put: &pb.PutResponse{
		Address: address,
		Status:  status,
		Error:   errMsg,
	}}}
}

// handleGet answers a download request. It returns only a reply write error.
func (cs *chunkStream) handleGet(streamCtx context.Context, req *pb.GetRequest) error {
	if len(req.Address) != swarm.HashSize {
		return cs.send(getResponse(req.Address, pb.Status_STATUS_BAD_REQUEST, nil, "invalid chunk address length"))
	}

	addr := swarm.NewAddress(req.Address)
	cache := cs.defaultCache
	switch req.Cache {
	case pb.CacheOption_CACHE_ENABLE:
		cache = true
	case pb.CacheOption_CACHE_DISABLE:
		cache = false
	}

	s := cs.s
	ctx, cancel := context.WithTimeout(streamCtx, s.chunkDownloadRequestTimeout)
	start := time.Now()
	chunk, err := s.storer.Download(cache).Get(ctx, addr)
	cancel()
	s.metrics.ChunkStreamFetchDuration.Observe(time.Since(start).Seconds())

	if err != nil {
		var (
			status = pb.Status_STATUS_ERROR
			errMsg string
		)
		switch {
		case errors.Is(err, storage.ErrNotFound):
			// Retrieval gave up after the peers it asked did not return the
			// chunk. It may still exist.
			status, errMsg = pb.Status_STATUS_NOT_FOUND, "chunk not found"
			s.metrics.ChunkStreamDeliveryCount.WithLabelValues("not_found").Inc()
			cs.logger.V(1).Build().Debug("chunk stream: chunk not found", "address", addr)
		case errors.Is(err, topology.ErrNotFound):
			// Retrieval ran out of peers to ask: none was usable, or every one
			// was tried. The chunk may or may not exist.
			status, errMsg = pb.Status_STATUS_UNAVAILABLE, "chunk could not be retrieved because no usable peer was available"
			s.metrics.ChunkStreamDeliveryCount.WithLabelValues("unavailable").Inc()
			cs.logger.V(1).Build().Debug("chunk stream: no usable peer for the chunk", "address", addr)
		case errors.Is(err, context.DeadlineExceeded):
			errMsg = "chunk retrieval timed out"
			s.metrics.ChunkStreamDeliveryCount.WithLabelValues("error").Inc()
			cs.logger.Debug("chunk stream: chunk retrieval timed out", "address", addr, "timeout", s.chunkDownloadRequestTimeout)
		default:
			errMsg = "read chunk failed"
			s.metrics.ChunkStreamDeliveryCount.WithLabelValues("error").Inc()
			cs.logger.Debug("chunk stream: read chunk failed", "address", addr, "error", err)
		}
		return cs.send(getResponse(req.Address, status, nil, errMsg))
	}

	s.metrics.ChunkStreamDeliveryCount.WithLabelValues("success").Inc()
	return cs.send(getResponse(req.Address, pb.Status_STATUS_OK, chunk.Data(), ""))
}

// chunkForAddress rebuilds an uploaded chunk and checks that it is the chunk the
// request names. The address also settles the chunk type: small SOC data parses
// as a valid CAC too, but only one reading of the data produces the address.
func chunkForAddress(address, data []byte) (swarm.Chunk, bool) {
	want := swarm.NewAddress(address)
	if ch, err := cac.NewWithDataSpan(data); err == nil && ch.Address().Equal(want) {
		return ch, true
	}
	// FromChunkValidate recovers the owner once and checks the SOC address
	// against the requested one.
	ch := swarm.NewChunk(want, data)
	if _, err := soc.FromChunkValidate(ch); err != nil {
		return nil, false
	}
	return ch, true
}

// handlePut answers an upload request. It returns only a reply write error.
func (cs *chunkStream) handlePut(streamCtx context.Context, req *pb.PutRequest) error {
	s := cs.s
	reply := func(status pb.Status, errMsg string) error {
		if status == pb.Status_STATUS_OK {
			s.metrics.ChunkStreamDeliveryCount.WithLabelValues("upload_success").Inc()
		} else {
			s.metrics.ChunkStreamDeliveryCount.WithLabelValues("upload_error").Inc()
		}
		return cs.send(putResponse(req.Address, status, errMsg))
	}

	if len(req.Address) != swarm.HashSize {
		return reply(pb.Status_STATUS_BAD_REQUEST, "invalid chunk address length")
	}
	if len(req.Data) < swarm.SpanSize {
		return reply(pb.Status_STATUS_BAD_REQUEST, "insufficient data for chunk")
	}

	chunk, ok := chunkForAddress(req.Address, req.Data)
	if !ok {
		return reply(pb.Status_STATUS_BAD_REQUEST, "address does not match chunk data")
	}

	// Stamping takes no context and spends a slot in the batch, so a request
	// still queued when the stream ends must not be stamped: its chunk would
	// never be pushed.
	select {
	case <-streamCtx.Done():
		return nil
	default:
	}

	stamp, err := cs.stamp(req, chunk)
	if err != nil {
		cs.logger.Debug("chunk stream: stamp failed", "address", chunk.Address(), "error", err)
		return reply(stampErrorReply(err))
	}

	// Uploads are always direct: the reply waits for the push to complete. The
	// wait has no time limit of its own, as with an HTTP direct upload. A push
	// carries on after its caller stops waiting, and the pusher answers a
	// repeated upload of a chunk still in flight with success at once, so a
	// timeout here could turn a retry of a timed-out chunk into STATUS_OK
	// before the chunk is pushed.
	session := s.storer.DirectUpload()
	if err := session.Put(streamCtx, chunk.WithStamp(stamp)); err != nil {
		cs.logger.Debug("chunk stream: direct upload put failed", "address", chunk.Address(), "error", err)
		return reply(pb.Status_STATUS_ERROR, "chunk write error")
	}
	if err := session.Done(swarm.ZeroAddress); err != nil {
		cs.logger.Debug("chunk stream: direct upload push failed", "address", chunk.Address(), "error", err)
		return reply(pb.Status_STATUS_ERROR, "chunk push failed")
	}
	return reply(pb.Status_STATUS_OK, "")
}

// Upload request errors. Each is answered with STATUS_BAD_REQUEST and its text.
var (
	errStreamMissingBatch       = errors.New("missing postage stamp or batch id")
	errStreamInvalidBatchID     = errors.New("invalid batch id")
	errStreamBatchMismatch      = errors.New("batch id differs from the connection batch")
	errStreamStampBatchMismatch = errors.New("stamp batch differs from the connection batch")
	errStreamInvalidStamp       = errors.New("invalid postage stamp")
	errStreamIdentityAddress    = errors.New("cannot compute identity address")

	streamRequestErrors = []error{
		errStreamMissingBatch,
		errStreamInvalidBatchID,
		errStreamBatchMismatch,
		errStreamStampBatchMismatch,
		errStreamInvalidStamp,
		errStreamIdentityAddress,
	}
)

// stampErrorReply returns the status and message that answer a stamping error.
func stampErrorReply(err error) (pb.Status, string) {
	for _, reqErr := range streamRequestErrors {
		if errors.Is(err, reqErr) {
			return pb.Status_STATUS_BAD_REQUEST, reqErr.Error()
		}
	}
	switch {
	case errors.Is(err, errBatchUnusable) || errors.Is(err, postage.ErrNotUsable):
		return pb.Status_STATUS_BAD_REQUEST, "batch not usable yet or does not exist"
	case errors.Is(err, postage.ErrNotFound) || errors.Is(err, storage.ErrNotFound):
		return pb.Status_STATUS_BAD_REQUEST, "batch with id not found"
	case errors.Is(err, postage.ErrBucketFull):
		return pb.Status_STATUS_ERROR, "batch is overissued"
	default:
		return pb.Status_STATUS_ERROR, "failed to stamp chunk"
	}
}

// stamp returns the stamp for an uploaded chunk: the stamp the client signed,
// or one the node signs with the request's batch or the connection's. Batches
// are looked up for every chunk, so one that expires while the stream is open
// is no longer used.
func (cs *chunkStream) stamp(req *pb.PutRequest, chunk swarm.Chunk) (*postage.Stamp, error) {
	if len(req.Stamp) > 0 {
		return cs.presignedStamp(req.Stamp, chunk.Address())
	}
	return cs.nodeStamp(req.BatchId, chunk)
}

// presignedStamp checks a stamp the client signed.
func (cs *chunkStream) presignedStamp(raw []byte, addr swarm.Address) (*postage.Stamp, error) {
	stamp := &postage.Stamp{}
	if err := stamp.UnmarshalBinary(raw); err != nil {
		return nil, fmt.Errorf("%w: %w", errStreamInvalidStamp, err)
	}
	// A connection batch pins pre-signed stamps too.
	if len(cs.defaultBatch) > 0 && !bytes.Equal(stamp.BatchID(), cs.defaultBatch) {
		return nil, errStreamStampBatchMismatch
	}

	batch, err := cs.s.batchStore.Get(stamp.BatchID())
	if err != nil {
		return nil, fmt.Errorf("get batch: %w", err)
	}
	// Valid checks the bucket and index as well as the signer, so a stamp the
	// network would reject is not pushed.
	if err := stamp.Valid(addr, batch.Owner, batch.Depth, batch.BucketDepth, batch.Immutable); err != nil {
		return nil, fmt.Errorf("%w: %w", errStreamInvalidStamp, err)
	}
	return stamp, nil
}

// nodeStamp stamps a chunk with the request's batch, or the connection's.
func (cs *chunkStream) nodeStamp(requestBatch []byte, chunk swarm.Chunk) (*postage.Stamp, error) {
	batchID := requestBatch
	if len(batchID) == 0 {
		batchID = cs.defaultBatch
	}
	if len(batchID) == 0 {
		return nil, errStreamMissingBatch
	}
	if len(batchID) != swarm.HashSize {
		return nil, errStreamInvalidBatchID
	}
	// A batch set on the connection pins it, so that a proxy which sets
	// Swarm-Postage-Batch-Id also decides the batch of every upload.
	if len(cs.defaultBatch) > 0 && !bytes.Equal(batchID, cs.defaultBatch) {
		return nil, errStreamBatchMismatch
	}

	idAddr, err := storage.IdentityAddress(chunk)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errStreamIdentityAddress, err)
	}

	stamper, save, err := cs.s.getStamper(batchID)
	if err != nil {
		return nil, fmt.Errorf("get stamper: %w", err)
	}
	stamp, err := stamper.Stamp(chunk.Address(), idAddr)
	if err != nil {
		return nil, fmt.Errorf("stamp chunk: %w", err)
	}
	if err := save(); err != nil {
		cs.logger.Debug("chunk stream: save stamp state failed", "error", err)
	}
	return stamp, nil
}
