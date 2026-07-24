// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package event defines the wire schema and fan-out bus that connect the
// headless simulation engine to the web UI. The sim layer knows nothing about
// this package; the Network translates sim hooks into the input events below
// and publishes them here.
package event

import (
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Wire message kinds (the "t" field).
const (
	KindHello  = "hello"
	KindSnap   = "snap"
	KindSync   = "sync"
	KindPut    = "put"
	KindMsg    = "msg"
	KindInject = "inject"
	KindRadius = "radius"
	KindConfig = "config"
)

// Edge sync modes derived from puller rules vs. radius.
const (
	ModeFull   = "full"
	ModePOOnly = "po-only"
	ModeIdle   = "idle"
)

// Directed-edge display states, ordered least to most active.
const (
	EdgeIdle       = "idle"
	EdgeCursors    = "cursors"
	EdgeAwaiting   = "awaiting-offer"
	EdgeOffer      = "offer-received"
	EdgeWant       = "want-sent"
	EdgeDelivering = "delivering"
)

// short renders an address as its first 8 hex characters for the wire.
func short(a swarm.Address) string {
	s := a.String()
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// Input events published into the Bus by the Network. Node references are
// resolved to indices before publishing.

// Put reports a newly stored chunk.
type Put struct {
	Node        int
	Addr        swarm.Address
	Bin         uint8
	BinID       uint64
	Source      string
	ReserveSize int
	Traced      bool
}

// Sync reports one completed client-side sync round (the ~1s coalescing wait
// included).
type Sync struct {
	From    int // syncing node
	Peer    int // upstream node
	Bin     uint8
	Start   uint64
	Topmost uint64
	Count   int
	DurMs   int64
	Err     string
}

// Msg reports one decoded protocol frame from the wire tap.
type Msg struct {
	Client   int // stream initiator (syncing puller)
	Server   int // responder
	Proto    string
	Stream   string
	StreamID uint64
	Dir      string
	Type     string
	Bin      uint8
	N        int
	Addr     swarm.Address
	HasAddr  bool
	Traced   bool
}

// StreamLC reports a stream lifecycle transition (open/close) with byte counts.
type StreamLC struct {
	Client   int
	Server   int
	Stream   string
	StreamID uint64
	Open     bool
	BytesC2S int64
	BytesS2C int64
}

// Inject reports an origin-seeding operation.
type Inject struct {
	Node   int
	Count  int
	Addrs  []swarm.Address
	Traced bool
}

// Radius reports a radius change on a node.
type Radius struct {
	Node   int
	Radius uint8
}

// Config reports a (re)build of the network.
type Config struct {
	Nodes     int    `json:"nodes"`
	Bins      uint8  `json:"bins"`
	Topology  string `json:"topology"`
	Degree    int    `json:"degree"`
	Radius    uint8  `json:"radius"`
	MaxPage   uint64 `json:"maxPage"`
	LatencyMs int64  `json:"latencyMs"`
	Clusters  int    `json:"clusters"`
	Seed      int64  `json:"seed"`
}

// Snapshot structures (the authoritative periodic frame).

// NodeSnap is the per-node view.
type NodeSnap struct {
	Index       int     `json:"index"`
	AddrPrefix  string  `json:"addrPrefix"`
	Angle       float64 `json:"angle"`
	Radius      uint8   `json:"radius"`
	ReserveSize int     `json:"reserveSize"`
	BinCounts   []int   `json:"binCounts"`
	HasTraced   bool    `json:"hasTraced"`
}

// EdgeSnap is the per-undirected-pair static view.
type EdgeSnap struct {
	A    int    `json:"a"`
	B    int    `json:"b"`
	PO   uint8  `json:"po"`
	Mode string `json:"mode"`
}

// StreamDetail describes one live stream on a directed edge.
type StreamDetail struct {
	StreamID  uint64 `json:"streamID"`
	Stream    string `json:"stream"`
	State     string `json:"state"`
	Bin       uint8  `json:"bin"`
	LastMsg   string `json:"lastMsg"`
	AgeMs     int64  `json:"ageMs"`
	Delivered int    `json:"delivered"`
	Total     int    `json:"total"`
}

// EdgeDirSnap is the per-directed-edge live state.
type EdgeDirSnap struct {
	From         int            `json:"from"` // syncing client
	To           int            `json:"to"`   // server
	State        string         `json:"state"`
	LastMsg      string         `json:"lastMsg"`
	LastMsgAgeMs int64          `json:"lastMsgAgeMs"`
	Counts       map[string]int `json:"counts"`
	Streams      []StreamDetail `json:"streams"`
	BytesC2S     int64          `json:"bytesC2S"`
	BytesS2C     int64          `json:"bytesS2C"`
}

// Stats is the global counters strip.
type Stats struct {
	Chunks      int     `json:"chunks"`
	SyncsPerSec float64 `json:"syncsPerSec"`
	Dropped     uint64  `json:"dropped"`
	Goroutines  int     `json:"goroutines"`
}

// Snapshot is the full authoritative state frame. Nodes/Edges/Stats.Chunks/
// Stats.Goroutines are filled by the Provider; the Bus fills EdgeDir and the
// remaining Stats fields.
type Snapshot struct {
	Nodes   []NodeSnap    `json:"nodes"`
	Edges   []EdgeSnap    `json:"edges"`
	EdgeDir []EdgeDirSnap `json:"edgeDir"`
	Stats   Stats         `json:"stats"`
}

// Provider supplies the base snapshot (nodes, edges, and node-derived stats).
type Provider interface {
	Snapshot() Snapshot
}

// ProviderFunc adapts a function to the Provider interface. It lets the web
// server swap the underlying network across rebuilds behind a stable bus.
type ProviderFunc func() Snapshot

// Snapshot implements Provider.
func (f ProviderFunc) Snapshot() Snapshot { return f() }
