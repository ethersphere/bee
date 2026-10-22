// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build integration

package node_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coreos/go-semver/semver"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
)

// This file is the transport and the topology of the acceptance suite (plan
// task T6.1): accNet carries the streams of every protocol between in-process
// nodes, as libp2p does between processes, and accTopology is each node's
// kademlia view of its connected peers.
//
// streamtest does not fit: a stream deadlocks once its writer is more than 16
// writes ahead of its reader (see the T6.2 follow-up), which pullsync pages of
// up to 250 deliveries and rangesync fetches of up to MaxWantIDs chunks reach
// at once, and its handlers run on context.Background, so a disconnect cannot
// end them. accNet's streams have libp2p's semantics instead, on the real
// clock: an unbounded buffer per direction (a writer never waits for its
// reader), Close ends this end's writes, FullClose also waits for the remote's
// end of the writes (closeDeadline, as pkg/p2p/libp2p), Reset fails both
// directions at once, and handlers run on a context that a disconnect cancels.

// Errors of accNet's streams, standing in for libp2p's.
var (
	errAccReset        = errors.New("acceptance net: stream reset")
	errAccClosed       = errors.New("acceptance net: stream closed")
	errAccExpectedEOF  = errors.New("acceptance net: read on full close: expected eof")
	errAccCloseTimeout = errors.New("acceptance net: full close deadline exceeded")
	errAccNotSupported = errors.New("acceptance net: protocol not supported")
	errAccNetClosed    = errors.New("acceptance net: closed")
)

// accCloseDeadline bounds FullClose's wait for the remote's end of the writes:
// libp2p's closeDeadline.
const accCloseDeadline = 30 * time.Second

// accNet is the in-process network: the hosts of the nodes and the
// connections between them. A connection is a full-duplex link on which
// either end opens streams.
type accNet struct {
	latency time.Duration // one-way latency of every stream

	ctx    context.Context // ends every handler at close
	cancel context.CancelFunc

	mu     sync.Mutex
	hosts  map[string]*accHost
	conns  map[[2]string]*accConn
	closed bool

	handlers sync.WaitGroup // running stream handlers

	disconnects atomic.Int64 // Disconnect and Blocklist calls of the protocols
	blocklists  atomic.Int64
	lastReason  atomic.Value // string: the reason of the last of them
}

func newAccNet(latency time.Duration) *accNet {
	ctx, cancel := context.WithCancel(context.Background())
	return &accNet{
		latency: latency,
		ctx:     ctx,
		cancel:  cancel,
		hosts:   make(map[string]*accHost),
		conns:   make(map[[2]string]*accConn),
	}
}

// newHost returns the host of overlay, whose topology is topo.
func (n *accNet) newHost(overlay swarm.Address, topo *accTopology) *accHost {
	h := &accHost{
		net:       n,
		overlay:   overlay,
		topo:      topo,
		protocols: make(map[string]p2p.ProtocolSpec),
		counters:  make(map[string]*accCounter),
	}
	n.mu.Lock()
	n.hosts[overlay.ByteString()] = h
	n.mu.Unlock()
	return h
}

func connKey(a, b swarm.Address) [2]string {
	k := [2]string{a.ByteString(), b.ByteString()}
	if k[0] > k[1] {
		k[0], k[1] = k[1], k[0]
	}
	return k
}

// connect connects a (the dialer) and b, as the libp2p service does after a
// handshake: the protocols' ConnectOut and ConnectIn hooks run, then each
// side's topology learns of the other.
func (n *accNet) connect(a, b *accHost) error {
	return n.connectWith(a, b, true)
}

// connectQuiet connects a and b without telling their topologies: a probe
// opens streams on it that no node's sync driver sees.
func (n *accNet) connectQuiet(a, b *accHost) error {
	return n.connectWith(a, b, false)
}

func (n *accNet) connectWith(a, b *accHost, notify bool) error {
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return errAccNetClosed
	}
	k := connKey(a.overlay, b.overlay)
	if _, ok := n.conns[k]; ok {
		n.mu.Unlock()
		return p2p.ErrAlreadyConnected
	}
	ctx, cancel := context.WithCancel(n.ctx)
	c := &accConn{dialer: a, listener: b, ctx: ctx, cancel: cancel, quiet: !notify, streams: make(map[*accStreamPair]struct{})}
	n.conns[k] = c
	n.mu.Unlock()

	for _, p := range a.specs() {
		if p.ConnectOut != nil {
			if err := p.ConnectOut(ctx, p2p.Peer{Address: b.overlay, FullNode: true}); err != nil {
				return fmt.Errorf("connect out %s: %w", p.Name, err)
			}
		}
	}
	for _, p := range b.specs() {
		if p.ConnectIn != nil {
			if err := p.ConnectIn(ctx, p2p.Peer{Address: a.overlay, FullNode: true}); err != nil {
				return fmt.Errorf("connect in %s: %w", p.Name, err)
			}
		}
	}
	if !notify {
		return nil
	}
	if err := a.topo.Connected(ctx, p2p.Peer{Address: b.overlay, FullNode: true}, false); err != nil {
		return err
	}
	return b.topo.Connected(ctx, p2p.Peer{Address: a.overlay, FullNode: true}, false)
}

// disconnect tears down the connection of a and b, as the libp2p service
// does: the topologies forget the peer, the protocols' disconnect hooks run,
// the handlers' context is canceled and every open stream is reset.
func (n *accNet) disconnect(a, b *accHost) error {
	n.mu.Lock()
	k := connKey(a.overlay, b.overlay)
	c, ok := n.conns[k]
	delete(n.conns, k)
	n.mu.Unlock()
	if !ok {
		return p2p.ErrPeerNotFound
	}
	c.cancel()
	c.resetAll()
	if !c.quiet {
		c.dialer.topo.Disconnected(p2p.Peer{Address: c.listener.overlay, FullNode: true})
		c.listener.topo.Disconnected(p2p.Peer{Address: c.dialer.overlay, FullNode: true})
	}
	for _, p := range c.dialer.specs() {
		if p.DisconnectOut != nil {
			_ = p.DisconnectOut(p2p.Peer{Address: c.listener.overlay, FullNode: true})
		}
	}
	for _, p := range c.listener.specs() {
		if p.DisconnectIn != nil {
			_ = p.DisconnectIn(p2p.Peer{Address: c.dialer.overlay, FullNode: true})
		}
	}
	return nil
}

func (n *accNet) conn(a, b swarm.Address) *accConn {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.conns[connKey(a, b)]
}

func (n *accNet) host(a swarm.Address) *accHost {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.hosts[a.ByteString()]
}

// close resets every stream, ends the handlers' context and waits for the
// handlers to return, at most timeout. It returns an error if some did not.
func (n *accNet) close(timeout time.Duration) error {
	n.mu.Lock()
	n.closed = true
	conns := make([]*accConn, 0, len(n.conns))
	for _, c := range n.conns {
		conns = append(conns, c)
	}
	n.mu.Unlock()
	n.cancel()
	for _, c := range conns {
		c.resetAll()
	}
	done := make(chan struct{})
	go func() {
		n.handlers.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return errors.New("acceptance net: stream handlers still running after close")
	}
}

// accConn is a connection between two hosts and its open streams.
type accConn struct {
	dialer, listener *accHost
	ctx              context.Context // the handlers' context
	cancel           context.CancelFunc
	quiet            bool // the topologies do not know of it (connectQuiet)

	mu      sync.Mutex
	streams map[*accStreamPair]struct{}
	reset   bool
}

func (c *accConn) add(p *accStreamPair) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.reset {
		return false
	}
	c.streams[p] = struct{}{}
	return true
}

func (c *accConn) remove(p *accStreamPair) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.streams, p)
}

func (c *accConn) resetAll() {
	c.mu.Lock()
	c.reset = true
	pairs := make([]*accStreamPair, 0, len(c.streams))
	for p := range c.streams {
		pairs = append(pairs, p)
	}
	clear(c.streams)
	c.mu.Unlock()
	for _, p := range pairs {
		_ = p.opener.Reset()
		_ = p.handler.Reset()
	}
}

// accCounter counts the bytes a host wrote (out) and the bytes written to it
// (in) on the streams of one protocol stream.
type accCounter struct {
	in, out atomic.Uint64
}

// accHost is the p2p service of one node: its protocols and its byte
// counters. It implements p2p.StreamerDisconnecter (and so p2p.Blocklister).
type accHost struct {
	net     *accNet
	overlay swarm.Address
	topo    *accTopology

	mu        sync.Mutex
	protocols map[string]p2p.ProtocolSpec // by name
	counters  map[string]*accCounter      // by "protocol/stream"
}

var _ p2p.StreamerDisconnecter = (*accHost)(nil)

// AddProtocol registers p, as the libp2p service's AddProtocol does.
func (h *accHost) AddProtocol(p p2p.ProtocolSpec) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.protocols[p.Name] = p
}

func (h *accHost) specs() []p2p.ProtocolSpec {
	h.mu.Lock()
	defer h.mu.Unlock()
	specs := make([]p2p.ProtocolSpec, 0, len(h.protocols))
	for _, p := range h.protocols {
		specs = append(specs, p)
	}
	return specs
}

// handler returns the handler of the stream, nil if this host does not
// support it.
func (h *accHost) handler(protocol, version, stream string) p2p.HandlerFunc {
	h.mu.Lock()
	defer h.mu.Unlock()
	p, ok := h.protocols[protocol]
	if !ok || p.Version != version {
		return nil
	}
	for _, s := range p.StreamSpecs {
		if s.Name == stream {
			return s.Handler
		}
	}
	return nil
}

func (h *accHost) counter(protocol, stream string) *accCounter {
	k := protocol + "/" + stream
	h.mu.Lock()
	defer h.mu.Unlock()
	c, ok := h.counters[k]
	if !ok {
		c = new(accCounter)
		h.counters[k] = c
	}
	return c
}

// bytes returns the bytes this host wrote (out) and that were written to it
// (in), by "protocol/stream".
func (h *accHost) bytes() (in, out map[string]uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	in, out = make(map[string]uint64), make(map[string]uint64)
	for k, c := range h.counters {
		in[k], out[k] = c.in.Load(), c.out.Load()
	}
	return in, out
}

// NewStream opens a stream to the connected peer addr, as the libp2p
// service's NewStream does: p2p.ErrPeerNotFound without a connection, an
// error wrapping p2p.IncompatibleStreamError if the peer does not support the
// stream. The peer's handler runs on the connection's context.
func (h *accHost) NewStream(ctx context.Context, addr swarm.Address, _ p2p.Headers, protocol, version, stream string) (p2p.Stream, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c := h.net.conn(h.overlay, addr)
	if c == nil {
		return nil, p2p.ErrPeerNotFound
	}
	remote := c.listener
	if remote == h {
		remote = c.dialer
	}
	handler := remote.handler(protocol, version, stream)
	if handler == nil {
		return nil, fmt.Errorf("new stream for peerid: %w", p2p.NewIncompatibleStreamError(errAccNotSupported))
	}
	v, err := semver.NewVersion(version)
	if err != nil {
		return nil, err
	}

	local, peer := h.counter(protocol, stream), remote.counter(protocol, stream)
	toHandler := newAccPipe(h.net.latency, func(n int) { local.out.Add(uint64(n)); peer.in.Add(uint64(n)) })
	toOpener := newAccPipe(h.net.latency, func(n int) { peer.out.Add(uint64(n)); local.in.Add(uint64(n)) })
	pair := &accStreamPair{conn: c}
	pair.opener = &accStream{pair: pair, in: toOpener, out: toHandler, version: v}
	pair.handler = &accStream{pair: pair, in: toHandler, out: toOpener, version: v}
	if !c.add(pair) {
		return nil, p2p.ErrPeerNotFound
	}

	h.net.handlers.Add(1)
	go func() {
		defer h.net.handlers.Done()
		err := handler(c.ctx, p2p.Peer{Address: h.overlay, FullNode: true}, pair.handler)
		if err == nil {
			return
		}
		// as the libp2p service's stream handler wrapper
		var de *p2p.DisconnectError
		if errors.As(err, &de) {
			_ = pair.handler.Reset()
			_ = remote.Disconnect(h.overlay, de.Error())
		}
		var bpe *p2p.BlockPeerError
		if errors.As(err, &bpe) {
			_ = pair.handler.Reset()
			_ = remote.Blocklist(h.overlay, bpe.Duration(), bpe.Error())
		}
	}()
	return pair.opener, nil
}

// Disconnect disconnects the peer overlay.
func (h *accHost) Disconnect(overlay swarm.Address, reason string) error {
	h.net.disconnects.Add(1)
	h.net.lastReason.Store(reason)
	peer := h.net.host(overlay)
	if peer == nil {
		return p2p.ErrPeerNotFound
	}
	return h.net.disconnect(h, peer)
}

// Blocklist disconnects the peer overlay. The suite does not reconnect
// peers, so it does not keep the blocklist itself.
func (h *accHost) Blocklist(overlay swarm.Address, _ time.Duration, reason string) error {
	h.net.blocklists.Add(1)
	h.net.lastReason.Store(reason)
	peer := h.net.host(overlay)
	if peer == nil {
		return p2p.ErrPeerNotFound
	}
	return h.net.disconnect(h, peer)
}

func (h *accHost) NetworkStatus() p2p.NetworkStatus { return p2p.NetworkStatusAvailable }

// accStreamPair is the two ends of a stream. It leaves its connection's set
// once both ends are closed.
type accStreamPair struct {
	conn            *accConn
	opener, handler *accStream
	closedEnds      atomic.Int32
}

func (p *accStreamPair) endClosed() {
	if p.closedEnds.Add(1) == 2 {
		p.conn.remove(p)
	}
}

// accStream is one end of a stream. Close ends this end's writes: the remote
// reads the bytes written, then io.EOF. FullClose also waits for the remote's
// end of the writes (at most accCloseDeadline), then ends this end's reads,
// and the remote's writes fail. Reset ends both directions: this end's reads
// and writes, and the remote's, fail with errAccReset, and the remote does not
// read the bytes it has not read yet. After any of them this end's reads and
// writes fail with errAccClosed.
type accStream struct {
	pair    *accStreamPair
	in, out *accPipe
	version *semver.Version

	mu     sync.Mutex
	closed bool
}

var _ p2p.Stream = (*accStream)(nil)

// markClosed marks this end closed and reports whether it was open.
func (s *accStream) markClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.closed = true
	s.pair.endClosed()
	return true
}

func (s *accStream) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *accStream) Read(b []byte) (int, error) {
	if s.isClosed() {
		return 0, errAccClosed
	}
	return s.in.read(b)
}

func (s *accStream) Write(b []byte) (int, error) {
	if s.isClosed() {
		return 0, errAccClosed
	}
	return s.out.write(b)
}

func (s *accStream) Close() error {
	if !s.markClosed() {
		return errAccClosed
	}
	s.out.closeWrite(false)
	return nil
}

func (s *accStream) FullClose() error {
	if !s.markClosed() {
		return errAccClosed
	}
	s.out.closeWrite(false)
	if err := s.in.awaitEOF(accCloseDeadline); err != nil {
		s.out.closeWrite(true)
		s.in.closeRead(errAccReset, errAccReset)
		return err
	}
	s.in.closeRead(io.EOF, errAccClosed)
	return nil
}

func (s *accStream) Reset() error {
	s.markClosed()
	s.out.closeWrite(true)
	s.in.closeRead(errAccReset, errAccReset)
	return nil
}

func (s *accStream) Headers() p2p.Headers         { return p2p.Headers{} }
func (s *accStream) ResponseHeaders() p2p.Headers { return p2p.Headers{} }

func (s *accStream) Version() (*semver.Version, error) { return s.version, nil }

// accPipe is one direction of a stream: an unbounded buffer, so that a writer
// never waits for its reader. With a latency, what the writer does (its
// bytes, the end of its writes, a reset) reaches the reader that much later,
// in order.
type accPipe struct {
	latency time.Duration
	onWrite func(n int) // counts the bytes written

	mu      sync.Mutex
	cond    sync.Cond
	buf     []byte       // arrived, unread
	pending []accArrival // written, on the way (latency)
	ended   bool         // the writer closed or reset (sent)
	eof     bool         // arrived: the writer closed
	reset   bool         // arrived: the writer reset the stream
	rerr    error        // the reader's end closed: what its reads return
	werr    error        // the reader's end closed: what writes return
}

// accArrival is what a writer did, arriving at a time.
type accArrival struct {
	at         time.Time
	data       []byte
	eof, reset bool
}

func newAccPipe(latency time.Duration, onWrite func(int)) *accPipe {
	p := &accPipe{latency: latency, onWrite: onWrite}
	p.cond.L = &p.mu
	return p
}

func (p *accPipe) write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.werr != nil {
		return 0, p.werr
	}
	if p.ended {
		return 0, errAccClosed
	}
	p.onWrite(len(b))
	p.sendLocked(accArrival{data: slices.Clone(b)})
	return len(b), nil
}

// closeWrite ends the writes: the reader reads io.EOF after the bytes
// written, or errAccReset at once (reset) once it arrives.
func (p *accPipe) closeWrite(reset bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ended && !reset {
		return
	}
	p.ended = true
	p.sendLocked(accArrival{eof: !reset, reset: reset})
}

func (p *accPipe) sendLocked(a accArrival) {
	if p.latency <= 0 {
		p.arriveLocked(a)
		return
	}
	a.at = time.Now().Add(p.latency)
	p.pending = append(p.pending, a)
	time.AfterFunc(p.latency, p.deliver)
}

// deliver delivers what has arrived by now.
func (p *accPipe) deliver() {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	n := 0
	for ; n < len(p.pending) && !p.pending[n].at.After(now); n++ {
		p.arriveLocked(p.pending[n])
	}
	clear(p.pending[:n])
	p.pending = p.pending[n:]
}

func (p *accPipe) arriveLocked(a accArrival) {
	switch {
	case a.reset:
		p.reset, p.buf = true, nil
	case a.eof:
		p.eof = true
	default:
		p.buf = append(p.buf, a.data...)
	}
	p.cond.Broadcast()
}

// closeRead ends the reader's end: its reads return rerr, the writer's writes
// werr.
func (p *accPipe) closeRead(rerr, werr error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.rerr == nil {
		p.rerr, p.werr = rerr, werr
	}
	p.cond.Broadcast()
}

func (p *accPipe) read(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for {
		switch {
		case p.reset:
			return 0, errAccReset
		case p.rerr != nil && (len(p.buf) == 0 || !errors.Is(p.rerr, io.EOF)):
			return 0, p.rerr
		case len(p.buf) > 0:
			n := copy(b, p.buf)
			if p.buf = p.buf[n:]; len(p.buf) == 0 {
				p.buf = nil
			}
			return n, nil
		case p.eof:
			return 0, io.EOF
		}
		p.cond.Wait()
	}
}

// awaitEOF waits until the writer's end of the writes arrives (FullClose). It
// returns errAccExpectedEOF if bytes arrive first or are unread, errAccReset
// on a reset, the reader's own close error, or errAccCloseTimeout once d has
// passed.
func (p *accPipe) awaitEOF(d time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	timedOut := false
	timer := time.AfterFunc(d, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		timedOut = true
		p.cond.Broadcast()
	})
	defer timer.Stop()
	for {
		switch {
		case p.reset:
			return errAccReset
		case len(p.buf) > 0:
			return errAccExpectedEOF
		case p.eof:
			return nil
		case p.rerr != nil:
			if errors.Is(p.rerr, io.EOF) {
				return nil
			}
			return p.rerr
		case timedOut:
			return errAccCloseTimeout
		}
		p.cond.Wait()
	}
}

// accTopology is a node's view of its connected peers, standing in for
// kademlia: the peers with their proximity order to the node, iterated by
// bin, and topology-change notifications on every connect and disconnect.
// The neighborhood depth is the storage radius the reserve sets
// (SetStorageRadius), as kademlia's is on a full node with enough peers.
type accTopology struct {
	base swarm.Address

	mu     sync.Mutex
	peers  []swarm.Address
	radius uint8

	subsMu sync.Mutex
	subs   []chan struct{}
}

var _ topology.Driver = (*accTopology)(nil)

func newAccTopology(base swarm.Address) *accTopology {
	return &accTopology{base: base}
}

func (t *accTopology) Connected(_ context.Context, peer p2p.Peer, _ bool) error {
	t.mu.Lock()
	if !slices.ContainsFunc(t.peers, peer.Address.Equal) {
		t.peers = append(t.peers, peer.Address)
	}
	t.mu.Unlock()
	t.trigger()
	return nil
}

func (t *accTopology) Disconnected(peer p2p.Peer) {
	t.mu.Lock()
	t.peers = slices.DeleteFunc(t.peers, peer.Address.Equal)
	t.mu.Unlock()
	t.trigger()
}

func (t *accTopology) Announce(context.Context, swarm.Address, bool) error { return nil }

func (t *accTopology) AnnounceTo(context.Context, swarm.Address, swarm.Address, bool) error {
	return nil
}

func (t *accTopology) AddPeers(...swarm.Address) {}

// ClosestPeer returns the connected peer closest to addr, or
// topology.ErrWantSelf if this node is closer and includeSelf is set.
func (t *accTopology) ClosestPeer(addr swarm.Address, includeSelf bool, _ topology.Select, skip ...swarm.Address) (swarm.Address, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	closer := func(x, y swarm.Address) bool {
		c, _ := swarm.DistanceCmp(addr, x, y)
		return c > 0
	}
	closest := swarm.ZeroAddress
	for _, p := range t.peers {
		if slices.ContainsFunc(skip, p.Equal) {
			continue
		}
		if closest.IsZero() || closer(p, closest) {
			closest = p
		}
	}
	if includeSelf && (closest.IsZero() || closer(t.base, closest)) {
		return swarm.ZeroAddress, topology.ErrWantSelf
	}
	if closest.IsZero() {
		return swarm.ZeroAddress, topology.ErrNotFound
	}
	return closest, nil
}

// byBin returns the connected peers with their proximity order, closest bin
// first.
func (t *accTopology) byBin() ([]swarm.Address, []uint8) {
	t.mu.Lock()
	peers := slices.Clone(t.peers)
	t.mu.Unlock()
	pos := make([]uint8, len(peers))
	for i, p := range peers {
		pos[i] = swarm.Proximity(t.base.Bytes(), p.Bytes())
	}
	idx := make([]int, len(peers))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool { return pos[idx[i]] > pos[idx[j]] })
	sp, spo := make([]swarm.Address, len(peers)), make([]uint8, len(peers))
	for i, k := range idx {
		sp[i], spo[i] = peers[k], pos[k]
	}
	return sp, spo
}

func eachByBin(peers []swarm.Address, pos []uint8, order []int, f topology.EachPeerFunc) error {
	skipBin, skipping := uint8(0), false
	for _, i := range order {
		if skipping && pos[i] == skipBin {
			continue
		}
		skipping = false
		stop, next, err := f(peers[i], pos[i])
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
		if next {
			skipBin, skipping = pos[i], true
		}
	}
	return nil
}

// EachConnectedPeer iterates the connected peers from the closest bin to the
// farthest.
func (t *accTopology) EachConnectedPeer(f topology.EachPeerFunc, _ topology.Select) error {
	peers, pos := t.byBin()
	order := make([]int, len(peers))
	for i := range order {
		order[i] = i
	}
	return eachByBin(peers, pos, order, f)
}

// EachConnectedPeerRev iterates the connected peers from the farthest bin to
// the closest.
func (t *accTopology) EachConnectedPeerRev(f topology.EachPeerFunc, _ topology.Select) error {
	peers, pos := t.byBin()
	order := make([]int, len(peers))
	for i := range order {
		order[i] = len(peers) - 1 - i
	}
	return eachByBin(peers, pos, order, f)
}

// SubscribeTopologyChange returns a channel that receives a signal after
// every connect, disconnect and radius change (coalesced), closed by
// unsubscribe, as kademlia's.
func (t *accTopology) SubscribeTopologyChange() (<-chan struct{}, func()) {
	c := make(chan struct{}, 1)
	t.subsMu.Lock()
	t.subs = append(t.subs, c)
	t.subsMu.Unlock()
	var once sync.Once
	return c, func() {
		once.Do(func() {
			t.subsMu.Lock()
			defer t.subsMu.Unlock()
			t.subs = slices.DeleteFunc(t.subs, func(s chan struct{}) bool { return s == c })
			close(c)
		})
	}
}

func (t *accTopology) trigger() {
	t.subsMu.Lock()
	defer t.subsMu.Unlock()
	for _, c := range t.subs {
		select {
		case c <- struct{}{}:
		default:
		}
	}
}

func (t *accTopology) peerCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.peers)
}

func (t *accTopology) NeighborhoodDepth() uint8 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.radius
}

// SetStorageRadius records the reserve's radius (the storer's RadiusSetter)
// and, if it changed, signals a topology change, as kademlia does.
func (t *accTopology) SetStorageRadius(r uint8) {
	t.mu.Lock()
	changed := t.radius != r
	t.radius = r
	t.mu.Unlock()
	if changed {
		t.trigger()
	}
}

func (t *accTopology) IsReachable() bool                                   { return true }
func (t *accTopology) UpdatePeerHealth(swarm.Address, bool, time.Duration) {}
func (t *accTopology) Snapshot() *topology.KadParams                       { return new(topology.KadParams) }
func (t *accTopology) Halt()                                               {}
func (t *accTopology) Close() error                                        { return nil }

// TestAcceptanceNet checks the transport of the acceptance suite: a writer
// far ahead of its reader neither blocks nor loses bytes (the streamtest
// deadlock), both ends' FullClose completes, a reset fails the remote's reads
// and writes, a stream the peer does not support fails with
// p2p.IncompatibleStreamError, one to an unconnected peer with
// p2p.ErrPeerNotFound, and a disconnect ends the handlers' context, resets the
// open streams and runs the topology and protocol hooks.
func TestAcceptanceNet(t *testing.T) {
	t.Parallel()

	net := newAccNet(0)
	a := net.newHost(swarm.RandAddress(t), nil)
	b := net.newHost(swarm.RandAddress(t), nil)
	a.topo, b.topo = newAccTopology(a.overlay), newAccTopology(b.overlay)

	const writes = 1000
	payload := []byte("0123456789abcdef")
	var (
		read         = make(chan int, 1)
		handlerCtx   = make(chan context.Context, 1)
		resetStream  = make(chan p2p.Stream, 1)
		disconnected atomic.Int32
	)
	b.AddProtocol(p2p.ProtocolSpec{
		Name:    "test",
		Version: "1.0.0",
		StreamSpecs: []p2p.StreamSpec{
			{Name: "bulk", Handler: func(_ context.Context, _ p2p.Peer, s p2p.Stream) error {
				for range writes {
					if _, err := s.Write(payload); err != nil {
						return err
					}
				}
				return s.FullClose()
			}},
			{Name: "count", Handler: func(_ context.Context, _ p2p.Peer, s p2p.Stream) error {
				n, err := io.Copy(io.Discard, s)
				read <- int(n)
				if err != nil {
					return err
				}
				return s.FullClose()
			}},
			{Name: "hold", Handler: func(ctx context.Context, _ p2p.Peer, s p2p.Stream) error {
				handlerCtx <- ctx
				resetStream <- s
				<-ctx.Done()
				return nil
			}},
		},
		DisconnectIn: func(p2p.Peer) error { disconnected.Add(1); return nil },
	})
	ctx := context.Background()

	if _, err := a.NewStream(ctx, b.overlay, nil, "test", "1.0.0", "bulk"); !errors.Is(err, p2p.ErrPeerNotFound) {
		t.Fatalf("stream to an unconnected peer: %v, want %v", err, p2p.ErrPeerNotFound)
	}
	if err := net.connect(a, b); err != nil {
		t.Fatal(err)
	}
	if a.topo.peerCount() != 1 || b.topo.peerCount() != 1 {
		t.Fatal("connect did not reach the topologies")
	}
	var ise *p2p.IncompatibleStreamError
	if _, err := a.NewStream(ctx, b.overlay, nil, "other", "1.0.0", "bulk"); !errors.As(err, &ise) {
		t.Fatalf("stream of an unsupported protocol: %v, want an IncompatibleStreamError", err)
	}
	if _, err := a.NewStream(ctx, b.overlay, nil, "test", "2.0.0", "bulk"); !errors.As(err, &ise) {
		t.Fatalf("stream of an unsupported version: %v, want an IncompatibleStreamError", err)
	}

	// the handler writes everything before the opener reads anything
	s, err := a.NewStream(ctx, b.overlay, nil, "test", "1.0.0", "bulk")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	got, err := io.ReadAll(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != writes*len(payload) {
		t.Fatalf("read %d bytes, want %d", len(got), writes*len(payload))
	}
	if err := s.FullClose(); err != nil {
		t.Fatal(err)
	}
	in, out := a.bytes()
	if in["test/bulk"] != uint64(len(got)) || out["test/bulk"] != 0 {
		t.Fatalf("bytes in %d out %d, want %d and 0", in["test/bulk"], out["test/bulk"], len(got))
	}

	// the opener writes and full-closes; the handler reads to EOF
	s, err = a.NewStream(ctx, b.overlay, nil, "test", "1.0.0", "count")
	if err != nil {
		t.Fatal(err)
	}
	for range writes {
		if _, err := s.Write(payload); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.FullClose(); err != nil {
		t.Fatal(err)
	}
	if n := <-read; n != writes*len(payload) {
		t.Fatalf("handler read %d bytes, want %d", n, writes*len(payload))
	}

	// a reset fails the remote's reads and writes
	s, err = a.NewStream(ctx, b.overlay, nil, "test", "1.0.0", "hold")
	if err != nil {
		t.Fatal(err)
	}
	remote := <-resetStream
	hctx := <-handlerCtx
	if err := s.Reset(); err != nil {
		t.Fatal(err)
	}
	if _, err := remote.Read(make([]byte, 1)); !errors.Is(err, errAccReset) {
		t.Fatalf("read after the remote's reset: %v, want %v", err, errAccReset)
	}
	if _, err := remote.Write(payload); !errors.Is(err, errAccReset) {
		t.Fatalf("write after the remote's reset: %v, want %v", err, errAccReset)
	}

	// a disconnect ends the handler's context and runs the hooks
	if err := a.Disconnect(b.overlay, "test"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-hctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the disconnect did not end the handler's context")
	}
	if a.topo.peerCount() != 0 || b.topo.peerCount() != 0 || disconnected.Load() != 1 {
		t.Fatalf("after the disconnect: peers %d and %d, hooks %d", a.topo.peerCount(), b.topo.peerCount(), disconnected.Load())
	}
	if err := net.close(5 * time.Second); err != nil {
		t.Fatal(err)
	}
}
