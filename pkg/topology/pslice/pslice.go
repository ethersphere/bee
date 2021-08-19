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
	peers     []swarm.Address // the slice of peers
	bins      []uint          // the indexes of every proximity order in the peers slice, index is po, value is index of peers slice
	baseBytes []byte
	sync.RWMutex
}

// New creates a new PSlice.
func New(maxBins int, base swarm.Address) *PSlice {
	return &PSlice{
		peers:     make([]swarm.Address, 0),
		bins:      make([]uint, maxBins),
		baseBytes: base.Bytes(),
	}
}

// iterates over all peers from deepest bin to shallowest.
func (s *PSlice) EachBin(pf topology.EachPeerFunc) error {
	s.RLock()
	peers, bins := s.peers, s.bins
	s.RUnlock()

	if len(peers) == 0 {
		return nil
	}

	var binEnd = uint(len(peers))

	for i := len(bins) - 1; i >= 0; i-- {
		for _, v := range peers[bins[i]:binEnd] {
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
		binEnd = bins[i]
	}

	return nil
}

// EachBinRev iterates over all peers from shallowest bin to deepest.
func (s *PSlice) EachBinRev(pf topology.EachPeerFunc) error {
	s.RLock()
	peers, bins := s.peers, s.bins
	s.RUnlock()

	if len(peers) == 0 {
		return nil
	}

	var binEnd int
	for i := 0; i <= len(bins)-1; i++ {
		if i == len(bins)-1 {
			binEnd = len(peers)
		} else {
			binEnd = int(bins[i+1])
		}

		for _, v := range peers[bins[i]:binEnd] {
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

func (s *PSlice) BinPeersLength(bin uint8) int {

	s.RLock()
	defer s.RUnlock()

	b := int(bin)
	if b >= len(s.bins) {
		return 0
	}

	var bEnd int
	if b == len(s.bins)-1 {
		bEnd = len(s.peers)
	} else {
		bEnd = int(s.bins[b+1])
	}

	return bEnd - int(s.bins[b])
}

func (s *PSlice) BinPeers(bin uint8) []swarm.Address {
	s.RLock()
	defer s.RUnlock()

	b := int(bin)
	if b >= len(s.bins) {
		return nil
	}

	var bEnd int
	if b == len(s.bins)-1 {
		bEnd = len(s.peers)
	} else {
		bEnd = int(s.bins[b+1])
	}

	ret := make([]swarm.Address, bEnd-int(s.bins[b]))
	copy(ret, s.peers[s.bins[b]:bEnd])

	return ret
}

func (s *PSlice) Length() int {
	s.RLock()
	defer s.RUnlock()

	return len(s.peers)
}

// ShallowestEmpty returns the shallowest empty bin if one exists.
// If such bin does not exists, returns true as bool value.
func (s *PSlice) ShallowestEmpty() (bin uint8, none bool) {
	s.RLock()
	defer s.RUnlock()

	binCp := make([]uint, len(s.bins)+1)
	copy(binCp, s.bins)
	binCp[len(binCp)-1] = uint(len(s.peers))

	for i := uint8(0); i < uint8(len(binCp)-1); i++ {
		if binCp[i+1] == binCp[i] {
			return i, false
		}
	}
	return 0, true
}

// Exists checks if a peer exists.
func (s *PSlice) Exists(addr swarm.Address) bool {
	s.RLock()
	defer s.RUnlock()

	b, _ := s.exists(addr)
	return b
}

// checks if a peer exists. must be called under lock.
func (s *PSlice) exists(addr swarm.Address) (bool, int) {
	if len(s.peers) == 0 {
		return false, 0
	}
	for i, a := range s.peers {
		if a.Equal(addr) {
			return true, i
		}
	}
	return false, 0
}

// Add a peer at a certain PO.
func (s *PSlice) Add(addrs ...swarm.Address) {
	s.Lock()
	defer s.Unlock()

	peers, bins := s.copy(len(addrs))

	for _, addr := range addrs {

		if e, _ := s.exists(addr); e {
			return
		}

		po := s.po(addr.Bytes())

		peers = insertAddresses(peers, int(s.bins[po]), addr)
		s.peers = peers

		incDeeper(bins, po)
		s.bins = bins
	}
}

// Remove a peer at a certain PO.
func (s *PSlice) Remove(addr swarm.Address) {
	s.Lock()
	defer s.Unlock()

	e, i := s.exists(addr)
	if !e {
		return
	}

	peers, bins := s.copy(0)

	peers = append(peers[:i], peers[i+1:]...)
	s.peers = peers

	decDeeper(bins, s.po(addr.Bytes()))
	s.bins = bins
}

func (s *PSlice) po(peer []byte) uint8 {

	po := swarm.Proximity(s.baseBytes, peer)
	if int(po) >= len(s.bins) {
		return uint8(len(s.bins)) - 1
	}

	return po
}

// incDeeper increments the peers slice bin index for proximity order > po for non-empty bins only.
// Must be called under lock.
func incDeeper(bins []uint, po uint8) {
	if po > uint8(len(bins)) {
		panic("po too high")
	}

	for i := po + 1; i < uint8(len(bins)); i++ {
		// don't increment if the value in k.bins == len(k.peers)
		// otherwise the calling context gets an out of bound error
		// when accessing the slice
		bins[i]++
	}
}

// decDeeper decrements the peers slice bin indexes for proximity order > po.
// Must be called under lock.
func decDeeper(bins []uint, po uint8) {
	if po > uint8(len(bins)) {
		panic("po too high")
	}

	for i := po + 1; i < uint8(len(bins)); i++ {
		bins[i]--
	}
}

// copy makes copies of peers and bins with a possibility of adding peers
// additional capacity if it is know that a number of new addresses will be
// inserted.
func (s *PSlice) copy(peersExtraCap int) (peers []swarm.Address, bins []uint) {
	peers = make([]swarm.Address, len(s.peers), len(s.peers)+peersExtraCap)
	copy(peers, s.peers)
	bins = make([]uint, len(s.bins))
	copy(bins, s.bins)
	return peers, bins
}

// insertAddresses is based on the optimized implementation from
// https://github.com/golang/go/wiki/SliceTricks#insertvector
func insertAddresses(s []swarm.Address, pos int, vs ...swarm.Address) []swarm.Address {
	if n := len(s) + len(vs); n <= cap(s) {
		s2 := s[:n]
		copy(s2[pos+len(vs):], s[pos:])
		copy(s2[pos:], vs)
		return s2
	}
	s2 := make([]swarm.Address, len(s)+len(vs))
	copy(s2, s[:pos])
	copy(s2[pos:], vs)
	copy(s2[pos+len(vs):], s[pos:])
	return s2
}
