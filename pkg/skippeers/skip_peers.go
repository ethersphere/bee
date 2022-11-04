// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package skippeers

import (
	"sync"

	"github.com/ethersphere/bee/pkg/swarm"
)

type List struct {
	mu                 sync.Mutex
	overdraftAddresses []swarm.Address
	addresses          []swarm.Address
}

func (s *List) All() []swarm.Address {
	s.mu.Lock()
	defer s.mu.Unlock()

	all := make([]swarm.Address, 0, len(s.addresses)+len(s.overdraftAddresses))
	all = append(all, s.addresses...)
	all = append(all, s.overdraftAddresses...)

	return all
}

func (s *List) ResetOverdraft() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.overdraftAddresses = nil
}

func (s *List) Add(address swarm.Address) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if swarm.AddressSliceContains(s.addresses, address) {
		return
	}

	s.addresses = append(s.addresses, address)
}

func (s *List) AddOverdraft(address swarm.Address) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if swarm.AddressSliceContains(s.overdraftAddresses, address) {
		return
	}

	for i, a := range s.addresses {
		if a.Equal(address) {
			s.addresses = append(s.addresses[:i], s.addresses[i+1:]...)
			break
		}
	}

	s.overdraftAddresses = append(s.overdraftAddresses, address)
}

// OverdraftListEmpty function returns whether all skipped entries a permanently skipped for this skiplist
// Temporary entries are stored in the overdraftAddresses slice of the skiplist, so if that is empty, the function returns true
func (s *List) OverdraftListEmpty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.overdraftAddresses) == 0
}
