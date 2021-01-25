// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package retrieval

import (
	"sync"

	"github.com/ethersphere/bee/pkg/swarm"
)

type skipPeers struct {
	addresses []swarm.Address
	mu        sync.Mutex
}

func newSkipPeers() *skipPeers {
	return &skipPeers{}
}

func (s *skipPeers) All() []swarm.Address {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append(s.addresses[:0:0], s.addresses...)
}

func (s *skipPeers) Add(address swarm.Address) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, a := range s.addresses {
		if a.Equal(address) {
			return
		}
	}

	s.addresses = append(s.addresses, address)
}
