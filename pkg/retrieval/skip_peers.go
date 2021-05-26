// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package retrieval

import (
	"sync"

	"github.com/ethersphere/bee/pkg/swarm"
)

type skipPeers struct {
	overdraftAddresses []swarm.Address
	addresses          []swarm.Address
	mu                 sync.Mutex
}

func newSkipPeers() *skipPeers {
	return &skipPeers{}
}

func (s *skipPeers) All() []swarm.Address {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append(append(s.addresses[:0:0], s.addresses...), s.overdraftAddresses...)
}

func (s *skipPeers) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.overdraftAddresses = []swarm.Address{}
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

func (s *skipPeers) AddOverdraft(address swarm.Address) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, a := range s.overdraftAddresses {
		if a.Equal(address) {
			return
		}
	}

	s.overdraftAddresses = append(s.overdraftAddresses, address)
}
