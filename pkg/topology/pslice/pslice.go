package pslice

import (
	"sync"

	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

type PSlice struct {
	peers     [][]swarm.Address // the slice of peers
	baseBytes []byte
	sync.RWMutex
}

func New(maxBins int, base swarm.Address) *PSlice {
	return &PSlice{
		peers:     make([][]swarm.Address, maxBins),
		baseBytes: base.Bytes(),
	}
}

func (s *PSlice) Add(addrs ...swarm.Address) {
	s.Lock()
	defer s.Unlock()

	for _, addr := range addrs {
		po := s.po(addr.Bytes())

		if e, _ := s.exists(addr, po); e {
			continue
		}

		s.peers[po] = append(s.peers[po], addr)
	}
}

// iterates over all peers from deepest bin to shallowest.
func (s *PSlice) EachBin(pf topology.EachPeerFunc) error {
	s.RLock()
	defer s.RUnlock()

	for i := len(s.peers) - 1; i >= 0; i-- {
		for _, peer := range s.peers[i] {
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
	defer s.RUnlock()

	for i := 0; i < len(s.peers); i++ {
		for _, peer := range s.peers[i] {
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

func (s *PSlice) po(peer []byte) uint8 {

	po := swarm.Proximity(s.baseBytes, peer)
	if int(po) >= len(s.peers) {
		return uint8(len(s.peers)) - 1
	}

	return po
}

// Exists checks if a peer exists.
func (s *PSlice) Exists(addr swarm.Address) bool {
	s.RLock()
	defer s.RUnlock()

	e, _ := s.exists(addr, s.po(addr.Bytes()))
	return e
}

// checks if a peer exists. must be called under lock.
func (s *PSlice) exists(addr swarm.Address, po uint8) (bool, int) {

	for i, peer := range s.peers[po] {
		if peer.Equal(addr) {
			return true, i
		}
	}

	return false, 0
}

// Remove a peer at a certain PO.
func (s *PSlice) Remove(addr swarm.Address) {
	s.Lock()
	defer s.Unlock()

	po := s.po(addr.Bytes())

	e, i := s.exists(addr, po)
	if !e {
		return
	}

	// replace index with last element
	s.peers[po][i] = s.peers[po][len(s.peers[po])-1]
	// reassign without last element
	s.peers[po] = s.peers[po][:len(s.peers[po])-1]
}
