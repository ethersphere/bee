// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sim

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"math/bits"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/pullsync/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Stream and message-type names inferred by the wire tap. These mirror the
// (unexported) pullsync protocol constants; they are stable across the
// protocol version this simulator targets. If the protocol reorders messages,
// the tap degrades to "unknown" frames rather than failing the stream.
const (
	streamNamePullsync = "pullsync"
	streamNameCursors  = "cursors"

	MsgGet      = "Get"
	MsgWant     = "Want"
	MsgOffer    = "Offer"
	MsgDelivery = "Delivery"
	MsgSyn      = "Syn"
	MsgAck      = "Ack"
	MsgUnknown  = "unknown"

	dirC2S = "c2s" // client (initiator) -> server (responder)
	dirS2C = "s2c" // server -> client
)

var (
	_ p2p.Streamer = (*Transport)(nil)
	_ p2p.Stream   = (*memStream)(nil)
)

var streamSeq atomic.Uint64

// MsgEvent is emitted for every decoded protocol frame seen on the wire.
type MsgEvent struct {
	Client   swarm.Address // stream initiator (the syncing puller)
	Server   swarm.Address // responder
	Proto    string
	Stream   string
	StreamID uint64
	Dir      string
	Type     string
	Bin      uint8
	N        int           // Offer: chunk count; Want: popcount; Delivery: 1-based index
	Addr     swarm.Address // Delivery chunk address, if any
}

// StreamPhase marks the lifecycle point of a StreamEvent.
type StreamPhase string

const (
	StreamOpen  StreamPhase = "open"
	StreamClose StreamPhase = "close"
)

// StreamEvent is emitted when a stream opens and when it closes (with byte
// counts populated on close).
type StreamEvent struct {
	Client   swarm.Address
	Server   swarm.Address
	Proto    string
	Stream   string
	StreamID uint64
	Phase    StreamPhase
	BytesC2S int64
	BytesS2C int64
}

// TransportHooks receives wire-level observability callbacks. Either field may
// be nil.
type TransportHooks struct {
	OnMsg    func(MsgEvent)
	OnStream func(StreamEvent)
}

// Transport is an in-memory p2p.Streamer for one node. Unlike streamtest it
// never sends on a bounded channel while holding a lock, so it does not
// deadlock on delivery bursts, and it cancels the server handler context when
// a stream is torn down from either side (matching libp2p reset semantics),
// freeing pullsync handlers parked in collectAddrs.
type Transport struct {
	base    swarm.Address
	latency time.Duration
	hooks   TransportHooks

	baseCtx    context.Context
	baseCancel context.CancelFunc

	mu       sync.Mutex
	handlers map[string]p2p.ProtocolSpec // key: dest addr.String()
	links    map[*link]struct{}
}

// NewTransport creates a transport for base. Handlers are registered with
// SetHandler after all peer Syncers exist and before Start.
func NewTransport(base swarm.Address, latency time.Duration, hooks TransportHooks) *Transport {
	ctx, cancel := context.WithCancel(context.Background())
	return &Transport{
		base:       base,
		latency:    latency,
		hooks:      hooks,
		baseCtx:    ctx,
		baseCancel: cancel,
		handlers:   make(map[string]p2p.ProtocolSpec),
		links:      make(map[*link]struct{}),
	}
}

// SetHandler registers the protocol spec of a destination peer.
func (t *Transport) SetHandler(dest swarm.Address, spec p2p.ProtocolSpec) {
	t.mu.Lock()
	t.handlers[dest.String()] = spec
	t.mu.Unlock()
}

// RemoveHandler drops the destination peer's protocol spec and tears down every
// live stream to it, which is what a peer disconnect looks like on the wire: no
// further dial can succeed, and any parked handler on the far side is released.
// It must not be called while the Network mutex is held, because closing a link
// publishes stream-lifecycle events back through the Network.
func (t *Transport) RemoveHandler(dest swarm.Address) {
	key := dest.String()
	t.mu.Lock()
	delete(t.handlers, key)
	ls := make([]*link, 0, len(t.links))
	for l := range t.links {
		if l.server.String() == key {
			ls = append(ls, l)
		}
	}
	t.mu.Unlock()
	for _, l := range ls {
		l.close()
	}
}

// NewStream dials the destination peer's handler for the named stream and
// returns the client end of an in-memory duplex stream.
func (t *Transport) NewStream(_ context.Context, address swarm.Address, _ p2p.Headers, protocol, _, stream string) (p2p.Stream, error) {
	t.mu.Lock()
	spec, ok := t.handlers[address.String()]
	t.mu.Unlock()
	if !ok {
		return nil, io.ErrClosedPipe
	}

	var handler p2p.HandlerFunc
	for _, ss := range spec.StreamSpecs {
		if ss.Name == stream {
			handler = ss.Handler
			break
		}
	}
	if handler == nil {
		return nil, io.ErrClosedPipe
	}

	id := streamSeq.Add(1)
	hctx, hcancel := context.WithCancel(t.baseCtx)

	l := &link{
		client:   t.base,
		server:   address,
		proto:    protocol,
		stream:   stream,
		id:       id,
		latency:  t.latency,
		hooks:    t.hooks,
		cancel:   hcancel,
		c2s:      newHalfConn(),
		s2c:      newHalfConn(),
		onClosed: func(l *link) { t.forget(l) },
	}
	l.startTaps()

	t.mu.Lock()
	t.links[l] = struct{}{}
	t.mu.Unlock()

	if t.hooks.OnStream != nil {
		t.hooks.OnStream(StreamEvent{
			Client: t.base, Server: address, Proto: protocol, Stream: stream,
			StreamID: id, Phase: StreamOpen,
		})
	}

	// Serve the peer's handler. The peer sees us (t.base) as the incoming peer.
	go func() {
		_ = handler(hctx, p2p.Peer{Address: t.base, FullNode: true}, l.serverStream())
		l.close()
	}()

	return l.clientStream(), nil
}

// Close tears down all live streams for this transport, cancelling parked
// handler contexts, and drops every registered handler so a later dial fails
// outright rather than opening a stream on an already-cancelled context.
func (t *Transport) Close() error {
	t.baseCancel()
	t.mu.Lock()
	clear(t.handlers)
	ls := make([]*link, 0, len(t.links))
	for l := range t.links {
		ls = append(ls, l)
	}
	t.mu.Unlock()
	for _, l := range ls {
		l.close()
	}
	return nil
}

func (t *Transport) forget(l *link) {
	t.mu.Lock()
	delete(t.links, l)
	t.mu.Unlock()
}

// link is the shared state of one bidirectional in-memory stream.
type link struct {
	client swarm.Address
	server swarm.Address
	proto  string
	stream string
	id     uint64

	latency time.Duration
	hooks   TransportHooks

	c2s *halfConn // client -> server
	s2c *halfConn // server -> client

	cancel    context.CancelFunc
	onClosed  func(*link)
	closeOnce sync.Once
	taps      sync.WaitGroup
}

func (l *link) startTaps() {
	if l.hooks.OnMsg == nil {
		return
	}
	l.taps.Add(2)
	go func() { defer l.taps.Done(); l.decode(l.c2s.tap, dirC2S) }()
	go func() { defer l.taps.Done(); l.decode(l.s2c.tap, dirS2C) }()
}

func (l *link) close() {
	l.closeOnce.Do(func() {
		l.c2s.close()
		l.s2c.close()
		// The taps decode asynchronously, so they lag the writers. Wait for
		// them to drain the already-buffered bytes before announcing the
		// close, otherwise consumers see messages for a stream they were
		// already told had ended. Closing the tap queues above makes the
		// decoders return once drained, so this cannot block indefinitely.
		l.taps.Wait()
		if l.cancel != nil {
			l.cancel()
		}
		if l.hooks.OnStream != nil {
			l.hooks.OnStream(StreamEvent{
				Client: l.client, Server: l.server, Proto: l.proto, Stream: l.stream,
				StreamID: l.id, Phase: StreamClose,
				BytesC2S: l.c2s.bytes.Load(), BytesS2C: l.s2c.bytes.Load(),
			})
		}
		if l.onClosed != nil {
			l.onClosed(l)
		}
	})
}

func (l *link) clientStream() p2p.Stream {
	return &memStream{link: l, read: l.s2c, write: l.c2s}
}

func (l *link) serverStream() p2p.Stream {
	return &memStream{link: l, read: l.c2s, write: l.s2c}
}

// decode reads varint-delimited protobuf frames from a tap and emits MsgEvents.
func (l *link) decode(q *byteQueue, dir string) {
	br := bufio.NewReader(q)
	frame := 0
	for {
		n, err := binary.ReadUvarint(br)
		if err != nil {
			return
		}
		payload := make([]byte, n)
		if _, err := io.ReadFull(br, payload); err != nil {
			return
		}
		frame++
		l.hooks.OnMsg(l.classify(dir, frame, payload))
	}
}

func (l *link) classify(dir string, frame int, payload []byte) MsgEvent {
	ev := MsgEvent{
		Client: l.client, Server: l.server, Proto: l.proto, Stream: l.stream,
		StreamID: l.id, Dir: dir, Type: MsgUnknown,
	}
	switch l.stream {
	case streamNameCursors:
		if dir == dirC2S {
			ev.Type = MsgSyn
		} else {
			ev.Type = MsgAck
		}
	case streamNamePullsync:
		if dir == dirC2S {
			if frame == 1 {
				ev.Type = MsgGet
				var g pb.Get
				if err := g.Unmarshal(payload); err == nil {
					ev.Bin = uint8(g.Bin)
				}
			} else {
				ev.Type = MsgWant
				var w pb.Want
				if err := w.Unmarshal(payload); err == nil {
					ev.N = popcount(w.BitVector)
				}
			}
		} else {
			if frame == 1 {
				ev.Type = MsgOffer
				var o pb.Offer
				if err := o.Unmarshal(payload); err == nil {
					ev.N = len(o.Chunks)
				}
			} else {
				ev.Type = MsgDelivery
				ev.N = frame - 1
				var d pb.Delivery
				if err := d.Unmarshal(payload); err == nil && len(d.Address) == swarm.HashSize {
					ev.Addr = swarm.NewAddress(d.Address)
				}
			}
		}
	}
	return ev
}

func popcount(b []byte) int {
	n := 0
	for _, x := range b {
		n += bits.OnesCount8(x)
	}
	return n
}

// halfConn is one direction of a stream: the consumer reads data, and a tap
// copy feeds the wire decoder.
type halfConn struct {
	data  *byteQueue
	tap   *byteQueue
	bytes atomic.Int64
}

func newHalfConn() *halfConn {
	return &halfConn{data: newByteQueue(), tap: newByteQueue()}
}

func (h *halfConn) write(p []byte, latency time.Duration) (int, error) {
	if latency > 0 {
		time.Sleep(latency)
	}
	h.bytes.Add(int64(len(p)))
	_, _ = h.tap.Write(p) // best-effort; ignore closed tap
	return h.data.Write(p)
}

func (h *halfConn) close() {
	h.data.Close(io.EOF)
	h.tap.Close(io.EOF)
}

// memStream is one endpoint's view of a link.
type memStream struct {
	link  *link
	read  *halfConn
	write *halfConn
}

func (s *memStream) Read(p []byte) (int, error)   { return s.read.data.Read(p) }
func (s *memStream) Write(p []byte) (int, error)  { return s.write.write(p, s.link.latency) }
func (s *memStream) Close() error                 { s.link.close(); return nil }
func (s *memStream) FullClose() error             { s.link.close(); return nil }
func (s *memStream) Reset() error                 { s.link.close(); return nil }
func (s *memStream) Headers() p2p.Headers         { return nil }
func (s *memStream) ResponseHeaders() p2p.Headers { return nil }

// byteQueue is an unbounded, blocking, single-buffer byte pipe. Writes never
// block; reads block until data or close. It never holds its lock across a
// blocking channel send, so it cannot deadlock the way streamtest can.
type byteQueue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	buf    bytes.Buffer
	closed bool
	err    error
}

func newByteQueue() *byteQueue {
	q := &byteQueue{}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *byteQueue) Write(p []byte) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return 0, io.ErrClosedPipe
	}
	n, _ := q.buf.Write(p)
	q.cond.Broadcast()
	return n, nil
}

func (q *byteQueue) Read(p []byte) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for q.buf.Len() == 0 && !q.closed {
		q.cond.Wait()
	}
	if q.buf.Len() == 0 && q.closed {
		if q.err != nil {
			return 0, q.err
		}
		return 0, io.EOF
	}
	return q.buf.Read(p)
}

func (q *byteQueue) Close(err error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.closed = true
	q.err = err
	q.cond.Broadcast()
}
