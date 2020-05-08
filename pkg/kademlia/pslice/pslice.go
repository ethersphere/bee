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
// Currently, when peers are added or removed, their proximity order must be supplied, this is
// in order to reduce duplicate PO calculation which is normally known and already needed in the
// calling context.
type PSlice struct {
	peers []swarm.Address
	bins  []uint

	sync.Mutex
}

// New creates a new PSlice.
func New(maxBins int) *PSlice {
	return &PSlice{
		peers: make([]swarm.Address, 0),
		bins:  make([]uint, maxBins),
	}
}

// iterates over all peers from deepest bin to shallowest.
func (s *PSlice) EachBin(pf topology.EachPeerFunc) error {
	s.Lock()
	defer s.Unlock()

	if len(s.peers) == 0 {
		return nil
	}

	var binEnd = uint(len(s.peers))

	for i := len(s.bins) - 1; i >= 0; i-- {
		peers := s.peers[s.bins[i]:binEnd]
		for _, v := range peers {
			stop, next, err := pf(v, uint8(i))
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
		binEnd = s.bins[i]
	}

	return nil
}

// EachBinRev iterates over all peers from shallowest to deepest.
func (s *PSlice) EachBinRev(pf topology.EachPeerFunc) error {
	s.Lock()
	defer s.Unlock()

	if len(s.peers) == 0 {
		return nil
	}

	var binEnd int
	for i := 0; i <= len(s.bins)-1; i++ {
		if i == len(s.bins)-1 {
			binEnd = len(s.peers)
		} else {
			binEnd = int(s.bins[i+1])
		}

		peers := s.peers[s.bins[i]:binEnd]
		for _, v := range peers {
			stop, next, err := pf(v, uint8(i))
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

// ShallowestEmpty returns the shallowest empty bin if one exists.
// If such bin does not exists, returns true as bool value.
// Must be called under lock.
func (s *PSlice) ShallowestEmpty() (bin uint8, none bool) {
	s.Lock()
	defer s.Unlock()

	binCp := make([]uint, len(s.bins)+1)
	copy(binCp, s.bins)
	binCp[len(binCp)-1] = uint(len(s.peers))

	for i := uint8(0); i < uint8(len(binCp)-1); i++ {
		if binCp[i+1]-binCp[i] == 0 {
			return i, false
		}
	}
	return 0, true
}

// Exists checks if a peer exists.
func (s *PSlice) Exists(addr swarm.Address) bool {
	s.Lock()
	defer s.Unlock()

	b, _ := s.exists(addr)
	return b
}

// checks if a peer exists. must be called under lock.
func (s *PSlice) exists(addr swarm.Address) (bool, int) {
	if len(s.peers) == 0 {
		return false, 0
	}
	return peerExists(s.peers, addr)
}

// Add a peer at a certain PO.
func (s *PSlice) Add(addr swarm.Address, po uint8) {
	s.Lock()
	defer s.Unlock()

	if e, _ := s.exists(addr); e {
		return
	}

	s.peers = append(s.peers[:s.bins[po]], append([]swarm.Address{addr}, s.peers[s.bins[po]:]...)...)
	s.incDeeper(po)
}

// Remove a peer at a certain PO.
func (s *PSlice) Remove(addr swarm.Address, po uint8) {
	s.Lock()
	defer s.Unlock()

	e, i := s.exists(addr)
	if !e {
		return
	}

	s.peers = append(s.peers[:i], s.peers[i+1:]...)
	s.decDeeper(po)
}

// incDeeper increments the peers slice bin index for proximity order > po.
// Must be called under lock.
func (s *PSlice) incDeeper(po uint8) {
	if po > uint8(len(s.bins)) {
		panic("po too high")
	}

	for i := po + 1; i < uint8(len(s.bins)); i++ {
		// don't increment if the value in k.bins == len(k.peers)
		// otherwise the calling context gets an out of bound error
		// when accessing the slice
		if s.bins[i] < uint(len(s.peers)) {
			s.bins[i]++
		}
	}
}

// decDeeper decrements the peers slice bin indexes for proximity order > po.
// Must be called under lock.
func (s *PSlice) decDeeper(po uint8) {
	if po > uint8(len(s.bins)) {
		panic("po too high")
	}

	for i := po + 1; i < uint8(len(s.bins)); i++ {
		s.bins[i]--
	}
}

// check if a certain address exists within the given slice of peers.
func peerExists(peers []swarm.Address, peer swarm.Address) (exists bool, idx int) {
	for i, addr := range peers {
		if addr.Equal(peer) {
			return true, i
		}
	}
	return false, 0
}
