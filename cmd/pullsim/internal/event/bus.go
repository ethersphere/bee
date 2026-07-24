// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	publishBuffer     = 4096
	subscriberBuffer  = 256
	snapshotInterval  = 250 * time.Millisecond
	maxDiscretePerSec = 200
	edgeStaleAfter    = 5 * time.Second
)

// Bus fans events out to subscribers and folds wire messages into per-directed
// -edge aggregate state. Snapshots are authoritative; discrete events are
// cosmetic, so a full publish channel drops (and counts) rather than blocking
// the protocol goroutines.
type Bus struct {
	provider Provider
	in       chan any
	quit     chan struct{}
	quitOnce sync.Once
	wg       sync.WaitGroup
	dropped  atomic.Uint64

	now func() time.Time

	subMu sync.Mutex
	subs  map[*subscriber]struct{}

	mu            sync.Mutex // guards the fields below
	edges         map[edgeKey]*edgeAgg
	traced        map[string]bool
	lastConfig    Config
	rlWindowStart time.Time
	rlCount       int
	syncsThisTick int
	syncsPerSec   float64
}

type subscriber struct {
	ch chan []byte
}

type edgeKey struct {
	client int
	server int
}

type edgeAgg struct {
	streams   map[uint64]*streamAgg
	lastMsg   string
	lastMsgAt time.Time
	bytesC2S  int64
	bytesS2C  int64
}

type streamAgg struct {
	name       string
	state      string
	bin        uint8
	offerTotal int
	deliveredN int
	lastMsg    string
	lastAt     time.Time
}

// NewBus creates a bus backed by the given base-snapshot provider.
func NewBus(provider Provider) *Bus {
	return &Bus{
		provider: provider,
		in:       make(chan any, publishBuffer),
		quit:     make(chan struct{}),
		now:      time.Now,
		subs:     make(map[*subscriber]struct{}),
		edges:    make(map[edgeKey]*edgeAgg),
		traced:   make(map[string]bool),
	}
}

// Start launches the drainer goroutine.
func (b *Bus) Start() {
	b.wg.Add(1)
	go b.run()
}

// Close stops the drainer and waits for it to exit.
func (b *Bus) Close() {
	b.quitOnce.Do(func() { close(b.quit) })
	b.wg.Wait()
}

// Publish enqueues an event. It never blocks: a full channel drops the event
// and increments the dropped counter.
func (b *Bus) Publish(ev any) {
	select {
	case b.in <- ev:
	default:
		b.dropped.Add(1)
	}
}

// Subscribe registers a new subscriber and immediately queues a hello frame.
// The returned channel delivers pre-encoded JSON frames; slow subscribers have
// frames dropped rather than blocking the bus.
func (b *Bus) Subscribe() (<-chan []byte, func()) {
	s := &subscriber{ch: make(chan []byte, subscriberBuffer)}

	// Queue the hello frame before registering so it is guaranteed to be the
	// first frame this subscriber receives, ahead of any concurrent broadcast.
	s.ch <- b.hello()

	b.subMu.Lock()
	b.subs[s] = struct{}{}
	b.subMu.Unlock()

	return s.ch, func() {
		b.subMu.Lock()
		if _, ok := b.subs[s]; ok {
			delete(b.subs, s)
			close(s.ch)
		}
		b.subMu.Unlock()
	}
}

func (b *Bus) run() {
	defer b.wg.Done()
	tick := time.NewTicker(snapshotInterval)
	defer tick.Stop()
	for {
		select {
		case ev := <-b.in:
			b.handle(ev)
		case <-tick.C:
			b.broadcast(b.snapshotFrame())
		case <-b.quit:
			return
		}
	}
}

func (b *Bus) handle(ev any) {
	switch e := ev.(type) {
	case Put:
		if frame, ok := b.encodePut(e); ok {
			b.broadcast(frame)
		}
	case Sync:
		b.mu.Lock()
		b.syncsThisTick++
		b.mu.Unlock()
		if e.Count > 0 {
			if frame, ok := b.encodeSync(e); ok {
				b.broadcast(frame)
			}
		}
	case Msg:
		b.foldMsg(e)
		if b.tracedAddr(e.Addr, e.HasAddr) || b.allow(false) {
			b.broadcast(b.encodeMsg(e))
		}
	case StreamLC:
		b.foldStream(e)
	case Inject:
		if e.Traced {
			b.mu.Lock()
			for _, a := range e.Addrs {
				b.traced[a.ByteString()] = true
			}
			b.mu.Unlock()
		}
		b.broadcast(b.encodeInject(e))
	case Radius:
		b.broadcast(b.encodeRadius(e))
	case Config:
		b.mu.Lock()
		b.lastConfig = e
		b.mu.Unlock()
		b.broadcast(b.encodeConfig(e))
	}
}

// allow applies the discrete-event rate limit (drainer-only; holds b.mu).
func (b *Bus) allow(traced bool) bool {
	if traced {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	if now.Sub(b.rlWindowStart) >= time.Second {
		b.rlWindowStart = now
		b.rlCount = 0
	}
	if b.rlCount >= maxDiscretePerSec {
		return false
	}
	b.rlCount++
	return true
}

func (b *Bus) tracedAddr(a swarm.Address, has bool) bool {
	if !has {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.traced[a.ByteString()]
}

func (b *Bus) foldMsg(e Msg) {
	b.mu.Lock()
	defer b.mu.Unlock()
	key := edgeKey{e.Client, e.Server}
	ea := b.edges[key]
	if ea == nil {
		ea = &edgeAgg{streams: make(map[uint64]*streamAgg)}
		b.edges[key] = ea
	}
	sa := ea.streams[e.StreamID]
	if sa == nil {
		sa = &streamAgg{name: e.Stream, state: EdgeIdle}
		ea.streams[e.StreamID] = sa
	}
	now := b.now()
	switch e.Type {
	case "Syn", "Ack":
		sa.state = EdgeCursors
	case "Get":
		sa.state = EdgeAwaiting
		sa.bin = e.Bin
	case "Offer":
		sa.state = EdgeOffer
		sa.offerTotal = e.N
	case "Want":
		sa.state = EdgeWant
	case "Delivery":
		sa.state = EdgeDelivering
		sa.deliveredN = e.N
	}
	sa.lastMsg = e.Type
	sa.lastAt = now
	ea.lastMsg = e.Type
	ea.lastMsgAt = now
}

func (b *Bus) foldStream(e StreamLC) {
	b.mu.Lock()
	defer b.mu.Unlock()
	key := edgeKey{e.Client, e.Server}
	ea := b.edges[key]
	if ea == nil {
		ea = &edgeAgg{streams: make(map[uint64]*streamAgg)}
		b.edges[key] = ea
	}
	if e.Open {
		if _, ok := ea.streams[e.StreamID]; !ok {
			init := EdgeIdle
			ea.streams[e.StreamID] = &streamAgg{name: e.Stream, state: init}
		}
	} else {
		delete(ea.streams, e.StreamID)
		ea.bytesC2S += e.BytesC2S
		ea.bytesS2C += e.BytesS2C
		ea.lastMsgAt = b.now()
	}
}

// stateRank orders edge display states by activity.
func stateRank(s string) int {
	switch s {
	case EdgeDelivering:
		return 5
	case EdgeWant:
		return 4
	case EdgeOffer:
		return 3
	case EdgeAwaiting:
		return 2
	case EdgeCursors:
		return 1
	default:
		return 0
	}
}

// buildEdgeDir folds live edge state into wire form and prunes stale edges.
// Must be called under b.mu.
func (b *Bus) buildEdgeDir() []EdgeDirSnap {
	now := b.now()
	out := make([]EdgeDirSnap, 0, len(b.edges))
	for key, ea := range b.edges {
		if len(ea.streams) == 0 && now.Sub(ea.lastMsgAt) > edgeStaleAfter {
			delete(b.edges, key)
			continue
		}
		counts := make(map[string]int)
		details := make([]StreamDetail, 0, len(ea.streams))
		best := EdgeIdle
		for id, sa := range ea.streams {
			counts[sa.state]++
			if stateRank(sa.state) > stateRank(best) {
				best = sa.state
			}
			details = append(details, StreamDetail{
				StreamID:  id,
				Stream:    sa.name,
				State:     sa.state,
				Bin:       sa.bin,
				LastMsg:   sa.lastMsg,
				AgeMs:     now.Sub(sa.lastAt).Milliseconds(),
				Delivered: sa.deliveredN,
				Total:     sa.offerTotal,
			})
		}
		out = append(out, EdgeDirSnap{
			From:         key.client,
			To:           key.server,
			State:        best,
			LastMsg:      ea.lastMsg,
			LastMsgAgeMs: now.Sub(ea.lastMsgAt).Milliseconds(),
			Counts:       counts,
			Streams:      details,
			BytesC2S:     ea.bytesC2S,
			BytesS2C:     ea.bytesS2C,
		})
	}
	return out
}

// snapshot assembles the full authoritative snapshot. Must be called under
// b.mu is NOT held; it acquires it internally.
func (b *Bus) snapshot() Snapshot {
	snap := b.provider.Snapshot()
	b.mu.Lock()
	// syncs/sec estimate: syncs observed since the previous tick, scaled.
	b.syncsPerSec = float64(b.syncsThisTick) * (float64(time.Second) / float64(snapshotInterval))
	b.syncsThisTick = 0
	snap.EdgeDir = b.buildEdgeDir()
	snap.Stats.Dropped = b.dropped.Load()
	snap.Stats.SyncsPerSec = b.syncsPerSec
	b.mu.Unlock()
	return snap
}

func (b *Bus) snapshotFrame() []byte {
	frame, _ := json.Marshal(struct {
		T  string `json:"t"`
		Ts int64  `json:"ts"`
		Snapshot
	}{T: KindSnap, Ts: b.now().UnixMilli(), Snapshot: b.snapshot()})
	return frame
}

func (b *Bus) hello() []byte {
	b.mu.Lock()
	cfg := b.lastConfig
	b.mu.Unlock()
	frame, _ := json.Marshal(struct {
		T      string   `json:"t"`
		Ts     int64    `json:"ts"`
		Config Config   `json:"config"`
		Snap   Snapshot `json:"snap"`
	}{T: KindHello, Ts: b.now().UnixMilli(), Config: cfg, Snap: b.snapshot()})
	return frame
}

func (b *Bus) broadcast(frame []byte) {
	if frame == nil {
		return
	}
	b.subMu.Lock()
	for s := range b.subs {
		select {
		case s.ch <- frame:
		default:
			// slow subscriber: drop this frame
		}
	}
	b.subMu.Unlock()
}

// Dropped returns the number of events dropped due to a full publish channel.
func (b *Bus) Dropped() uint64 { return b.dropped.Load() }

// --- discrete encoders ---

func (b *Bus) encodePut(e Put) ([]byte, bool) {
	if !b.tracedAddr(e.Addr, true) && !b.allow(e.Traced) {
		return nil, false
	}
	return mustJSON(struct {
		T           string `json:"t"`
		Ts          int64  `json:"ts"`
		Node        int    `json:"node"`
		Addr        string `json:"addr"`
		Bin         uint8  `json:"bin"`
		BinID       uint64 `json:"binID"`
		Source      string `json:"source"`
		ReserveSize int    `json:"reserveSize"`
	}{KindPut, b.now().UnixMilli(), e.Node, short(e.Addr), e.Bin, e.BinID, e.Source, e.ReserveSize}), true
}

func (b *Bus) encodeSync(e Sync) ([]byte, bool) {
	if !b.allow(false) {
		return nil, false
	}
	return mustJSON(struct {
		T       string `json:"t"`
		Ts      int64  `json:"ts"`
		From    int    `json:"from"`
		Peer    int    `json:"peer"`
		Bin     uint8  `json:"bin"`
		Start   uint64 `json:"start"`
		Topmost uint64 `json:"topmost"`
		Count   int    `json:"count"`
		DurMs   int64  `json:"durMs"`
		Err     string `json:"err,omitempty"`
	}{KindSync, b.now().UnixMilli(), e.From, e.Peer, e.Bin, e.Start, e.Topmost, e.Count, e.DurMs, e.Err}), true
}

func (b *Bus) encodeMsg(e Msg) []byte {
	addr := ""
	if e.HasAddr {
		addr = short(e.Addr)
	}
	return mustJSON(struct {
		T        string `json:"t"`
		Ts       int64  `json:"ts"`
		Client   int    `json:"client"`
		Server   int    `json:"server"`
		Proto    string `json:"proto"`
		Stream   string `json:"stream"`
		StreamID uint64 `json:"streamID"`
		Dir      string `json:"dir"`
		Type     string `json:"type"`
		Bin      uint8  `json:"bin"`
		N        int    `json:"n"`
		Addr     string `json:"addr,omitempty"`
	}{KindMsg, b.now().UnixMilli(), e.Client, e.Server, e.Proto, e.Stream, e.StreamID, e.Dir, e.Type, e.Bin, e.N, addr})
}

func (b *Bus) encodeInject(e Inject) []byte {
	addrs := make([]string, len(e.Addrs))
	for i, a := range e.Addrs {
		addrs[i] = short(a)
	}
	return mustJSON(struct {
		T     string   `json:"t"`
		Ts    int64    `json:"ts"`
		Node  int      `json:"node"`
		Count int      `json:"count"`
		Addrs []string `json:"addrs"`
	}{KindInject, b.now().UnixMilli(), e.Node, e.Count, addrs})
}

func (b *Bus) encodeRadius(e Radius) []byte {
	return mustJSON(struct {
		T      string `json:"t"`
		Ts     int64  `json:"ts"`
		Node   int    `json:"node"`
		Radius uint8  `json:"radius"`
	}{KindRadius, b.now().UnixMilli(), e.Node, e.Radius})
}

func (b *Bus) encodeConfig(e Config) []byte {
	return mustJSON(struct {
		T      string `json:"t"`
		Ts     int64  `json:"ts"`
		Config Config `json:"config"`
	}{KindConfig, b.now().UnixMilli(), e})
}

func mustJSON(v any) []byte {
	out, _ := json.Marshal(v)
	return out
}
