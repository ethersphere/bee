// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package full

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/discovery"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var _ topology.Driver = (*driver)(nil)

// driver drives the connectivity between nodes. It is a basic implementation of a connectivity driver.
// that enabled full connectivity in the sense that:
// - Every peer which is added to the driver gets broadcasted to every other peer regardless of its address.
// - A random peer is picked when asking for a peer to retrieve an arbitrary chunk (Peerer interface).
type driver struct {
	base          swarm.Address // the base address for this node
	discovery     discovery.Driver
	addressBook   addressbook.Interface
	p2pService    p2p.Service
	receivedPeers map[string]struct{} // track already received peers. Note: implement cleanup or expiration if needed to stop infinite grow
	backoffActive bool
	logger        logging.Logger
	mtx           sync.Mutex
	quit          chan struct{}
}

func New(disc discovery.Driver, addressBook addressbook.Interface, p2pService p2p.Service, logger logging.Logger, baseAddress swarm.Address) topology.Driver {
	return &driver{
		base:          baseAddress,
		discovery:     disc,
		addressBook:   addressBook,
		p2pService:    p2pService,
		receivedPeers: make(map[string]struct{}),
		logger:        logger,
		quit:          make(chan struct{}),
	}
}

// AddPeer adds a new peer to the topology driver.
// The peer would be subsequently broadcasted to all connected peers.
// All connected peers are also broadcasted to the new peer.
func (d *driver) AddPeer(ctx context.Context, addr swarm.Address) error {
	d.mtx.Lock()
	if _, ok := d.receivedPeers[addr.ByteString()]; ok {
		d.mtx.Unlock()
		return nil
	}

	d.receivedPeers[addr.ByteString()] = struct{}{}
	d.mtx.Unlock()
	connectedPeers := d.p2pService.Peers()
	bzzAddress, err := d.addressBook.Get(addr)
	if err != nil {
		if err == addressbook.ErrNotFound {
			return topology.ErrNotFound
		}
		return err
	}

	if !isConnected(addr, connectedPeers) {
		_, err := d.p2pService.Connect(ctx, bzzAddress.Underlay, true)
		if err != nil {
			d.mtx.Lock()
			delete(d.receivedPeers, addr.ByteString())
			d.mtx.Unlock()
			var e *p2p.ConnectionBackoffError
			if errors.As(err, &e) {
				d.backoff(e.TryAfter())
				return err
			}
			return err
		}
	}

	connectedAddrs := []swarm.Address{}
	for _, addressee := range connectedPeers {
		// skip newly added peer
		if addressee.Address.Equal(addr) {
			continue
		}

		connectedAddrs = append(connectedAddrs, addressee.Address)
		if err := d.discovery.BroadcastPeers(ctx, addressee.Address, addr); err != nil {
			return err
		}
	}

	if len(connectedAddrs) == 0 {
		return nil
	}

	return d.discovery.BroadcastPeers(ctx, addr, connectedAddrs...)
}

// ClosestPeer returns the closest connected peer we have in relation to a
// given chunk address. Returns topology.ErrWantSelf in case base is the closest to the chunk.
func (d *driver) ClosestPeer(addr swarm.Address) (swarm.Address, error) {
	connectedPeers := d.p2pService.Peers()
	if len(connectedPeers) == 0 {
		return swarm.Address{}, topology.ErrNotFound
	}

	// start checking closest from _self_
	closest := d.base
	for _, peer := range connectedPeers {
		dcmp, err := swarm.DistanceCmp(addr.Bytes(), closest.Bytes(), peer.Address.Bytes())
		if err != nil {
			return swarm.Address{}, err
		}
		switch dcmp {
		case 0:
			// do nothing
		case -1:
			// current peer is closer
			closest = peer.Address
		case 1:
			// closest is already closer to chunk
			// do nothing
		}
	}

	// check if self
	if closest.Equal(d.base) {
		return swarm.Address{}, topology.ErrWantSelf
	}

	return closest, nil
}

func (d *driver) Connected(ctx context.Context, addr swarm.Address) error {
	return d.AddPeer(ctx, addr)
}

func (_ *driver) Disconnected(swarm.Address) {
	// TODO: implement if necessary
}

func (_ *driver) NeighborhoodDepth() uint8 {
	return 0
}

// EachPeer iterates from closest bin to farthest
func (_ *driver) EachPeer(_ topology.EachPeerFunc) error {
	panic("not implemented") // TODO: Implement
}

// EachPeerRev iterates from farthest bin to closest
func (_ *driver) EachPeerRev(_ topology.EachPeerFunc) error {
	panic("not implemented") // TODO: Implement
}

func (_ *driver) SubscribePeersChange() (c <-chan struct{}, unsubscribe func()) {
	//TODO implement if necessary
	return c, unsubscribe
}

func (d *driver) MarshalJSON() ([]byte, error) {
	var peers []string
	for p := range d.receivedPeers {
		peers = append(peers, p)
	}
	return json.Marshal(struct {
		Peers []string `json:"peers"`
	}{Peers: peers})
}

func (d *driver) String() string {
	return fmt.Sprintf("%s", d.receivedPeers)
}

func (d *driver) Close() error {
	close(d.quit)
	return nil
}

func (d *driver) backoff(tryAfter time.Time) {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	if d.backoffActive {
		return
	}

	d.backoffActive = true
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer cancel()
		select {
		case <-done:
		case <-d.quit:
		}
	}()

	go func() {
		defer func() { close(done) }()
		select {
		case <-time.After(time.Until(tryAfter)):
			d.mtx.Lock()
			d.backoffActive = false
			d.mtx.Unlock()
			addresses, _ := d.addressBook.Overlays()
			for _, addr := range addresses {
				select {
				case <-d.quit:
					return
				default:
					if err := d.AddPeer(ctx, addr); err != nil {
						var e *p2p.ConnectionBackoffError
						if errors.As(err, &e) {
							d.backoff(e.TryAfter())
							return
						}
					}
				}
			}
		case <-d.quit:
			return
		}
	}()
}

func isConnected(addr swarm.Address, connectedPeers []p2p.Peer) bool {
	for _, p := range connectedPeers {
		if p.Address.Equal(addr) {
			return true
		}
	}

	return false
}
