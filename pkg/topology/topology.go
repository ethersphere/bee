// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package topology exposes abstractions needed in
// topology-aware components.
package topology

import (
	"errors"
	"io"
	"time"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	ErrNotFound      = errors.New("no peer found")
	ErrWantSelf      = errors.New("node wants self")
	ErrOversaturated = errors.New("oversaturated")
)

type Driver interface {
	p2p.Notifier
	PeerAdder
	ClosestPeerer
	EachPeerer
	EachNeighbor
	NeighborhoodDepther
	SubscribePeersChange() (c <-chan struct{}, unsubscribe func())
	io.Closer
	Halter
	Snapshot() *KadParams
}

type PeerAdder interface {
	// AddPeers is called when peers are added to the topology backlog
	AddPeers(addr ...swarm.Address)
}

type ClosestPeerer interface {
	// ClosestPeer returns the closest connected peer we have in relation to a
	// given chunk address.
	// This function will ignore peers with addresses provided in skipPeers.
	// Returns topology.ErrWantSelf in case base is the closest to the address.
	ClosestPeer(addr swarm.Address, includeSelf bool, skipPeers ...swarm.Address) (peerAddr swarm.Address, err error)
}

type EachPeerer interface {
	// EachPeer iterates from closest bin to farthest
	EachPeer(EachPeerFunc) error
	// EachPeerRev iterates from farthest bin to closest
	EachPeerRev(EachPeerFunc) error
}

type EachNeighbor interface {
	// EachNeighbor iterates from closest bin to farthest within the neighborhood.
	EachNeighbor(EachPeerFunc) error
	// EachNeighborRev iterates from farthest bin to closest within the neighborhood.
	EachNeighborRev(EachPeerFunc) error
	// IsWithinDepth checks if an address is the within neighborhood.
	IsWithinDepth(swarm.Address) bool
}

// EachPeerFunc is a callback that is called with a peer and its PO
type EachPeerFunc func(swarm.Address, uint8) (stop, jumpToNext bool, err error)

// PeerInfo is a view of peer information exposed to a user.
type PeerInfo struct {
	Address swarm.Address       `json:"address"`
	Metrics *MetricSnapshotView `json:"metrics,omitempty"`
}

// MetricSnapshotView represents snapshot of metrics counters in more human readable form.
type MetricSnapshotView struct {
	LastSeenTimestamp          int64   `json:"lastSeenTimestamp"`
	SessionConnectionRetry     uint64  `json:"sessionConnectionRetry"`
	ConnectionTotalDuration    float64 `json:"connectionTotalDuration"`
	SessionConnectionDuration  float64 `json:"sessionConnectionDuration"`
	SessionConnectionDirection string  `json:"sessionConnectionDirection"`
	LatencyEWMA                int64   `json:"latencyEWMA"`
	Reachability               string  `json:"reachability"`
}

type BinInfo struct {
	BinPopulation     uint        `json:"population"`
	BinConnected      uint        `json:"connected"`
	DisconnectedPeers []*PeerInfo `json:"disconnectedPeers"`
	ConnectedPeers    []*PeerInfo `json:"connectedPeers"`
}

type KadBins struct {
	Bin0  BinInfo `json:"bin_0"`
	Bin1  BinInfo `json:"bin_1"`
	Bin2  BinInfo `json:"bin_2"`
	Bin3  BinInfo `json:"bin_3"`
	Bin4  BinInfo `json:"bin_4"`
	Bin5  BinInfo `json:"bin_5"`
	Bin6  BinInfo `json:"bin_6"`
	Bin7  BinInfo `json:"bin_7"`
	Bin8  BinInfo `json:"bin_8"`
	Bin9  BinInfo `json:"bin_9"`
	Bin10 BinInfo `json:"bin_10"`
	Bin11 BinInfo `json:"bin_11"`
	Bin12 BinInfo `json:"bin_12"`
	Bin13 BinInfo `json:"bin_13"`
	Bin14 BinInfo `json:"bin_14"`
	Bin15 BinInfo `json:"bin_15"`
	Bin16 BinInfo `json:"bin_16"`
	Bin17 BinInfo `json:"bin_17"`
	Bin18 BinInfo `json:"bin_18"`
	Bin19 BinInfo `json:"bin_19"`
	Bin20 BinInfo `json:"bin_20"`
	Bin21 BinInfo `json:"bin_21"`
	Bin22 BinInfo `json:"bin_22"`
	Bin23 BinInfo `json:"bin_23"`
	Bin24 BinInfo `json:"bin_24"`
	Bin25 BinInfo `json:"bin_25"`
	Bin26 BinInfo `json:"bin_26"`
	Bin27 BinInfo `json:"bin_27"`
	Bin28 BinInfo `json:"bin_28"`
	Bin29 BinInfo `json:"bin_29"`
	Bin30 BinInfo `json:"bin_30"`
	Bin31 BinInfo `json:"bin_31"`
}

type KadParams struct {
	Base           string    `json:"baseAddr"`       // base address string
	Population     int       `json:"population"`     // known
	Connected      int       `json:"connected"`      // connected count
	Timestamp      time.Time `json:"timestamp"`      // now
	NNLowWatermark int       `json:"nnLowWatermark"` // low watermark for depth calculation
	Depth          uint8     `json:"depth"`          // current depth
	Bins           KadBins   `json:"bins"`           // individual bin info
	Reachability   string    `json:"reachability"`   // current reachability status
	LightNodes     BinInfo   `json:"lightNodes"`     // light nodes bin info
}

type Halter interface {
	// Halt the topology from initiating new connections
	// while allowing it to still run.
	Halt()
}

type NeighborhoodDepther interface {
	NeighborhoodDepth() uint8
}
