// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"github.com/ethersphere/bee/pkg/swarm"
)

type MockPricer struct{}

func NewPricer() *MockPricer {
	return &MockPricer{}
}

func (pricer *MockPricer) PeerPrice(peer, chunk swarm.Address) uint64 {
	return 0
}

func (pricer *MockPricer) Price(chunk swarm.Address) uint64 {
	return 0
}
