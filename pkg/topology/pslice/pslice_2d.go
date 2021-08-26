package pslice

import (
	"sync"

	"github.com/ethersphere/bee/pkg/swarm"
)

type PSlice2D struct {
	peers     [][]swarm.Address // the slice of peers
	baseBytes []byte

	sync.RWMutex
}

func New2D(maxBins int, base swarm.Address) *PSlice2D {
	return &PSlice2D{
		peers:     make([][]swarm.Address, maxBins),
		baseBytes: base.Bytes(),
	}
}

func (s *PSlice2D) Add(addrs ...swarm.Address) {
	s.Lock()
	defer s.Unlock()

	for _, addr := range addrs {
		po := s.po(addr.Bytes())

		exists := false
		for _, peer := range s.peers[po] {
			if addr.Equal(peer) {
				exists = true
				break
			}
		}

		if exists {
			continue
		}

		s.peers[po] = append(s.peers[po], addr)
	}

	// peers, bins := s.copy(len(addrs))

	// for _, addr := range addrs {

	// 	if e, _ := s.exists(addr); e {
	// 		return
	// 	}

	// 	po := s.po(addr.Bytes())

	// 	peers = insertAddresses(peers, int(s.bins[po]), addr)
	// 	s.peers = peers

	// 	incDeeper(bins, po)
	// 	s.bins = bins
	// }
}

func (s *PSlice2D) po(peer []byte) uint8 {

	po := swarm.Proximity(s.baseBytes, peer)
	if int(po) >= len(s.peers) {
		return uint8(len(s.peers)) - 1
	}

	return po
}

// var (
// 	base = test.RandomAddress()
// 	ps   = pslice.New(16, base)
// )

// for i := 0; i < 16; i++ {
// 	for j := 0; j < 300; j++ {
// 		ps.Add(test.RandomAddressAt(base, i))
// 	}
// }

// const po = 8

// b.ResetTimer()
// for n := 0; n < b.N; n++ {
// 	ps.Add(test.RandomAddressAt(base, po))
// }
