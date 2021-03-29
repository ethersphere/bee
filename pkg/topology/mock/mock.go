// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"sync"

	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

type mock struct {
	peers           []swarm.Address
	closestPeer     swarm.Address
	closestPeerErr  error
	peersErr        error
	addPeersErr     error
	marshalJSONFunc func() ([]byte, error)
	mtx             sync.Mutex
}

func WithPeers(peers ...swarm.Address) Option {
	return optionFunc(func(d *mock) {
		d.peers = peers
	})
}

func WithAddPeersErr(err error) Option {
	return optionFunc(func(d *mock) {
		d.addPeersErr = err
	})
}

func WithClosestPeer(addr swarm.Address) Option {
	return optionFunc(func(d *mock) {
		d.closestPeer = addr
	})
}

func WithClosestPeerErr(err error) Option {
	return optionFunc(func(d *mock) {
		d.closestPeerErr = err
	})
}

func WithMarshalJSONFunc(f func() ([]byte, error)) Option {
	return optionFunc(func(d *mock) {
		d.marshalJSONFunc = f
	})
}

func NewTopologyDriver(opts ...Option) topology.Driver {
	d := new(mock)
	for _, o := range opts {
		o.apply(d)
	}
	return d
}

func (d *mock) AddPeers(_ context.Context, addrs ...swarm.Address) error {
	if d.addPeersErr != nil {
		return d.addPeersErr
	}

	d.mtx.Lock()
	defer d.mtx.Unlock()

	d.peers = append(d.peers, addrs...)

	return nil
}

func (d *mock) Connected(ctx context.Context, addr swarm.Address) error {
	return d.AddPeers(ctx, addr)
}

func (d *mock) Disconnected(swarm.Address) {
	panic("todo")
}

func (d *mock) Peers() []swarm.Address {
	return d.peers
}

func (d *mock) ClosestPeer(adr swarm.Address, skipPeers ...swarm.Address) (peerAddr swarm.Address, err error) {
	if len(skipPeers) == 0 {
		if d.closestPeerErr != nil {
			return d.closestPeer, d.closestPeerErr
		}
		if !d.closestPeer.Equal(swarm.ZeroAddress) {
			return d.closestPeer, nil
		}
	}

	d.mtx.Lock()
	defer d.mtx.Unlock()

	if len(d.peers) == 0 {
		return peerAddr, topology.ErrNotFound
	}

	skipPeer := false
	for _, p := range d.peers {
		for _, a := range skipPeers {
			if a.Equal(p) {
				skipPeer = true
				break
			}
		}
		if skipPeer {
			skipPeer = false
			continue
		}

		if peerAddr.IsZero() {
			peerAddr = p
		}

		if cmp, _ := swarm.DistanceCmp(adr.Bytes(), p.Bytes(), peerAddr.Bytes()); cmp == 1 {
			peerAddr = p
		}
	}

	if peerAddr.IsZero() {
		return peerAddr, topology.ErrNotFound
	}
	return peerAddr, nil
}

func (d *mock) SubscribePeersChange() (c <-chan struct{}, unsubscribe func()) {
	return c, unsubscribe
}

func (*mock) NeighborhoodDepth() uint8 {
	return 0
}

func (m *mock) IsWithinDepth(adr swarm.Address) bool {
	return false
}

func (m *mock) EachNeighbor(f topology.EachPeerFunc) error {
	return m.EachPeer(f)
}

func (*mock) EachNeighborRev(topology.EachPeerFunc) error {
	panic("not implemented") // TODO: Implement
}

// EachPeer iterates from closest bin to farthest
func (d *mock) EachPeer(f topology.EachPeerFunc) (err error) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	if d.peersErr != nil {
		return d.peersErr
	}

	for i, p := range d.peers {
		_, _, err = f(p, uint8(i))
		if err != nil {
			return
		}
	}

	return nil
}

// EachPeerRev iterates from farthest bin to closest
func (d *mock) EachPeerRev(f topology.EachPeerFunc) (err error) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	for i := len(d.peers) - 1; i >= 0; i-- {
		_, _, err = f(d.peers[i], uint8(i))
		if err != nil {
			return
		}
	}

	return nil
}

func (d *mock) Snapshot() *topology.KadParams {
	return new(topology.KadParams)
}

func (d *mock) Close() error {
	return nil
}

type Option interface {
	apply(*mock)
}

type optionFunc func(*mock)

func (f optionFunc) apply(r *mock) { f(r) }
