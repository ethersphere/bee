// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
)

type AddrTuple struct {
	Addr swarm.Address // the peer address
	PO   uint8         // the po
}

func WithEachPeerRevCalls(addrs ...AddrTuple) Option {
	return optionFunc(func(m *Mock) {
		m.eachPeerRev = append(m.eachPeerRev, addrs...)
	})
}

func WithDepth(d uint8) Option {
	return optionFunc(func(m *Mock) {
		m.depth = d
	})
}

func WithDepthCalls(d ...uint8) Option {
	return optionFunc(func(m *Mock) {
		m.depthReplies = d
	})
}

type Mock struct {
	mtx          sync.Mutex
	peers        []swarm.Address
	eachPeerRev  []AddrTuple
	depth        uint8
	depthReplies []uint8
	depthCalls   int
	trigs        []chan struct{}
	trigMtx      sync.Mutex
}

func NewMockKademlia(o ...Option) *Mock {
	m := &Mock{}
	for _, v := range o {
		v.apply(m)
	}

	return m
}

// AddPeers is called when a peers are added to the topology backlog
// for further processing by connectivity strategy.
func (m *Mock) AddPeers(addr ...swarm.Address) {
	panic("not implemented") // TODO: Implement
}

func (m *Mock) ClosestPeer(addr swarm.Address, _ bool, _ topology.Select, skipPeers ...swarm.Address) (peerAddr swarm.Address, err error) {
	panic("not implemented") // TODO: Implement
}

func (m *Mock) EachNeighbor(topology.EachPeerFunc) error {
	panic("not implemented") // TODO: Implement
}

func (m *Mock) EachNeighborRev(topology.EachPeerFunc) error {
	panic("not implemented") // TODO: Implement
}

func (m *Mock) UpdatePeerHealth(swarm.Address, bool, time.Duration) {
	panic("not implemented") // TODO: Implement
}

// PeerIterator iterates from closest bin to farthest
func (m *Mock) SetStorageRadius(uint8) {
	panic("not implemented")
}

func (m *Mock) AddRevPeers(addrs ...AddrTuple) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.eachPeerRev = append(m.eachPeerRev, addrs...)
}

// EachConnectedPeer iterates from closest bin to farthest
func (m *Mock) EachConnectedPeer(f topology.EachPeerFunc, _ topology.Select) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	for i := len(m.peers) - 1; i > 0; i-- {
		stop, _, err := f(m.peers[i], uint8(i))
		if stop {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// EachPeerRev iterates from farthest bin to closest
func (m *Mock) EachConnectedPeerRev(f topology.EachPeerFunc, _ topology.Select) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	for _, v := range m.eachPeerRev {
		stop, _, err := f(v.Addr, v.PO)
		if stop {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Mock) IsReachable() bool {
	return true
}

func (m *Mock) NeighborhoodDepth() uint8 {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	m.depthCalls++
	if len(m.depthReplies) > 0 {
		return m.depthReplies[m.depthCalls]
	}
	return m.depth
}

// Connected is called when a peer dials in.
func (m *Mock) Connected(_ context.Context, peer p2p.Peer, _ bool) error {
	m.mtx.Lock()
	m.peers = append(m.peers, peer.Address)
	m.mtx.Unlock()
	m.Trigger()
	return nil
}

// Disconnected is called when a peer disconnects.
func (m *Mock) Disconnected(peer p2p.Peer) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	m.peers = swarm.RemoveAddress(m.peers, peer.Address)

	m.Trigger()
}

func (m *Mock) Announce(_ context.Context, _ swarm.Address, _ bool) error {
	return nil
}

func (m *Mock) AnnounceTo(_ context.Context, _, _ swarm.Address, _ bool) error {
	return nil
}

func (m *Mock) SubscribeTopologyChange() (c <-chan struct{}, unsubscribe func()) {
	channel := make(chan struct{}, 1)
	var closeOnce sync.Once

	m.trigMtx.Lock()
	defer m.trigMtx.Unlock()
	m.trigs = append(m.trigs, channel)

	unsubscribe = func() {
		m.trigMtx.Lock()
		defer m.trigMtx.Unlock()

		for i, c := range m.trigs {
			if c == channel {
				m.trigs = append(m.trigs[:i], m.trigs[i+1:]...)
				break
			}
		}

		closeOnce.Do(func() { close(channel) })
	}

	return channel, unsubscribe
}

func (m *Mock) Trigger() {
	m.trigMtx.Lock()
	defer m.trigMtx.Unlock()

	for _, c := range m.trigs {
		select {
		case c <- struct{}{}:
		default:
		}
	}
}

func (m *Mock) ResetPeers() {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.peers = nil
	m.eachPeerRev = nil
}

func (d *Mock) Halt()        {}
func (m *Mock) Close() error { return nil }

func (m *Mock) Snapshot() *topology.KadParams {
	panic("not implemented") // TODO: Implement
}

type Option interface {
	apply(*Mock)
}
type optionFunc func(*Mock)

func (f optionFunc) apply(r *Mock) { f(r) }
