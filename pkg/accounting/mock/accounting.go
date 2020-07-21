// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"github.com/ethersphere/bee/pkg/swarm"
	"sync"
)

type MockAccounting struct {
	lock     sync.Mutex
	balances map[string]int64
}

func (ma *MockAccounting) Reserve(peer swarm.Address, price uint64) error {
	return nil
}

func (ma *MockAccounting) Release(peer swarm.Address, price uint64) {

}

func (ma *MockAccounting) Credit(peer swarm.Address, price uint64) error {
	ma.lock.Lock()
	defer ma.lock.Unlock()
	ma.balances[peer.String()] -= int64(price)
	return nil
}

func (ma *MockAccounting) Debit(peer swarm.Address, price uint64) error {
	ma.lock.Lock()
	defer ma.lock.Unlock()
	ma.balances[peer.String()] += int64(price)
	return nil
}

func (ma *MockAccounting) Balance(peer swarm.Address) (int64, error) {
	ma.lock.Lock()
	defer ma.lock.Unlock()
	return ma.balances[peer.String()], nil
}

func NewAccounting() *MockAccounting {
	return &MockAccounting{
		balances: make(map[string]int64),
	}
}
