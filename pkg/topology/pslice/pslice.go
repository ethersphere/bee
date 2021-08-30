// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pslice

import (
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

// PSlice maintains a list of addresses, indexing them by their different proximity orders.
// Currently, when peers are added or removed, their proximity order must be supplied, this is
// in order to reduce duplicate PO calculation which is normally known and already needed in the
// calling context.
type PSlice struct {
	peers     [][]swarm.Address // the slice of peers
	baseBytes []byte
	sync.RWMutex
}

// New creates a new PSlice
func New(maxBins int, base swarm.Address) *PSlice {
	return &PSlice{
		peers:     make([][]swarm.Address, maxBins),
		baseBytes: base.Bytes(),
	}
}

// Add a peer at a certain PO.
func (s *PSlice) Add(addrs ...swarm.Address) {
	s.Lock()
	defer s.Unlock()

	pos := make([]uint8, 0, len(addrs))
	poCount := make([]int, len(s.peers))

	for _, addr := range addrs {
		po := s.po(addr.Bytes())
		fmt.Println(po)
		pos = append(pos, po)
		poCount[po]++
	}

	for i, count := range poCount {
		peers := s.peers[i]
		if count > 0 && cap(peers) < len(peers)+count {
			newPeers := make([]swarm.Address, len(peers), len(peers)+count)
			copy(newPeers, peers)
			s.peers[i] = newPeers
		}
	}

	for i, addr := range addrs {
		po := pos[i]

		if e, _ := s.index(addr, po); e {
			continue
		}

		s.peers[po] = append(s.peers[po], addr)
	}
}

// iterates over all peers from deepest bin to shallowest.
func (s *PSlice) EachBin(pf topology.EachPeerFunc) error {
	s.RLock()
	peers := s.peers
	s.RUnlock()

	for i := len(peers) - 1; i >= 0; i-- {
		for _, peer := range peers[i] {
			stop, next, err := pf(peer, uint8(i))
			if err != nil {
				return err
			}
			if stop {
				return nil
			}
			if next {
				break
			}
		}
	}

	return nil
}

// EachBinRev iterates over all peers from shallowest bin to deepest.
func (s *PSlice) EachBinRev(pf topology.EachPeerFunc) error {
	s.RLock()
	peers := s.peers
	s.RUnlock()

	for i := 0; i < len(peers); i++ {
		for _, peer := range peers[i] {
			stop, next, err := pf(peer, uint8(i))
			if err != nil {
				return err
			}
			if stop {
				return nil
			}
			if next {
				break
			}
		}
	}
	return nil
}

// BinPeers returns the slice of peers in a certain bin
func (s *PSlice) BinPeers(bin uint8) []swarm.Address {
	s.RLock()
	defer s.RUnlock()

	if int(bin) >= len(s.peers) {
		return nil
	}

	ret := make([]swarm.Address, len(s.peers[bin]))
	copy(ret, s.peers[bin])

	return ret
}

// Length returns the number of peers in the Pslice
func (s *PSlice) Length() int {
	s.RLock()
	defer s.RUnlock()

	var ret int

	for _, peers := range s.peers {
		ret += len(peers)
	}

	return ret
}

// ShallowestEmpty returns the shallowest empty bin if one exists.
// If such bin does not exists, returns true as bool value.
func (s *PSlice) ShallowestEmpty() (uint8, bool) {
	s.RLock()
	defer s.RUnlock()

	for i, peers := range s.peers {

		if len(peers) == 0 {
			return uint8(i), false
		}

	}

	return 0, true
}

// Exists checks if a peer exists.
func (s *PSlice) Exists(addr swarm.Address) bool {
	s.RLock()
	defer s.RUnlock()

	e, _ := s.index(addr, s.po(addr.Bytes()))
	return e
}

// Remove a peer at a certain PO.
func (s *PSlice) Remove(addr swarm.Address) {
	s.Lock()
	defer s.Unlock()

	po := s.po(addr.Bytes())

	e, i := s.index(addr, po)
	if !e {
		return
	}

	// replace index with last element
	s.peers[po][i] = s.peers[po][len(s.peers[po])-1]
	// reassign without last element
	s.peers[po] = s.peers[po][:len(s.peers[po])-1]
}

func (s *PSlice) po(peer []byte) uint8 {
	po := swarm.Proximity(s.baseBytes, peer)
	if int(po) >= len(s.peers) {
		return uint8(len(s.peers)) - 1
	}
	return po
}

// checks if a peer exists. must be called under lock.
func (s *PSlice) index(addr swarm.Address, po uint8) (bool, int) {

	for i, peer := range s.peers[po] {
		if peer.Equal(addr) {
			return true, i
		}
	}

	return false, 0
}
