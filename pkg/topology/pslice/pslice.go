// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pslice

import (
	"sync"

	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

// PSlice maintains a list of addresses, indexing them by their different proximity orders.
type PSlice struct {
	peers     [][]swarm.Address // the slice of peers
	baseBytes []byte
	sync.RWMutex
	maxBins int
}

// New creates a new PSlice.
func New(maxBins int, base swarm.Address) *PSlice {
	return &PSlice{
		peers:     make([][]swarm.Address, maxBins),
		baseBytes: base.Bytes(),
		maxBins:   maxBins,
	}
}

// Add a peer at a certain PO.
func (s *PSlice) Add(addrs ...swarm.Address) {
	s.Lock()
	defer s.Unlock()

	// bypass unnecessary allocations below if address count is one
	if len(addrs) == 1 {
		addr := addrs[0]
		po := s.po(addr.Bytes())
		if e, _ := s.index(addr, po); e {
			return
		}
		s.peers[po] = append(s.peers[po], addr)
		return
	}

	addrPo := make([]uint8, 0, len(addrs))
	binChange := make([]int, s.maxBins)
	exists := make([]bool, len(addrs))

	for i, addr := range addrs {
		po := s.po(addr.Bytes())
		addrPo = append(addrPo, po)
		if e, _ := s.index(addr, po); e {
			exists[i] = true
		} else {
			binChange[po]++
		}
	}

	for i, count := range binChange {
		peers := s.peers[i]
		if count > 0 && cap(peers) < len(peers)+count {
			newPeers := make([]swarm.Address, len(peers), len(peers)+count)
			copy(newPeers, peers)
			s.peers[i] = newPeers
		}
	}

	for i, addr := range addrs {
		if exists[i] {
			continue
		}

		po := addrPo[i]
		s.peers[po] = append(s.peers[po], addr)
	}
}

// iterates over all peers from deepest bin to shallowest.
func (s *PSlice) EachBin(pf topology.EachPeerFunc) error {

	s.RLock()
	pcopy := make([][]swarm.Address, s.maxBins)
	for i := range pcopy {
		pcopy[i] = s.peers[i]
	}
	s.RUnlock()

	for i := s.maxBins - 1; i >= 0; i-- {

		for _, peer := range pcopy[i] {
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
	pcopy := make([][]swarm.Address, s.maxBins)
	for i := range pcopy {
		pcopy[i] = s.peers[i]
	}
	s.RUnlock()

	for i := 0; i < s.maxBins; i++ {

		for _, peer := range pcopy[i] {

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

func (s *PSlice) BinSize(bin uint8) int {

	if int(bin) >= s.maxBins {
		return 0
	}

	s.RLock()
	defer s.RUnlock()

	return len(s.peers[bin])
}

func (s *PSlice) BinPeers(bin uint8) []swarm.Address {

	if int(bin) >= s.maxBins {
		return nil
	}

	s.RLock()
	defer s.RUnlock()

	ret := make([]swarm.Address, len(s.peers[bin]))
	copy(ret, s.peers[bin])

	return ret
}

// Length returns the number of peers in the Pslice.
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

	// Since order of elements does not matter, the optimized removing process
	// below replaces the index to be removed with the last element of the array,
	// and shortens the array by one.

	// make copy of the bin slice with one fewer element
	newLength := len(s.peers[po]) - 1
	cpy := make([]swarm.Address, newLength)
	copy(cpy, s.peers[po][:newLength])

	// if the index is the last element, then assign slice and return early
	if i == newLength {
		s.peers[po] = cpy
		return
	}

	// replace index being removed with last element
	lastItem := s.peers[po][newLength]
	cpy[i] = lastItem

	// assign the copy with the index removed back to the original array
	s.peers[po] = cpy
}

func (s *PSlice) po(peer []byte) uint8 {
	po := swarm.Proximity(s.baseBytes, peer)
	if int(po) >= s.maxBins {
		return uint8(s.maxBins) - 1
	}
	return po
}

// index returns if a peer exists and the index in the slice.
func (s *PSlice) index(addr swarm.Address, po uint8) (bool, int) {

	for i, peer := range s.peers[po] {
		if peer.Equal(addr) {
			return true, i
		}
	}

	return false, 0
}
