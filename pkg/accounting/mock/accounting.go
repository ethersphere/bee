// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"github.com/ethersphere/bee/pkg/swarm"
)

type MockAccounting struct {
}

func (ma *MockAccounting) Reserve(peer swarm.Address, price uint64) error {
	return nil
}

func (ma *MockAccounting) Release(peer swarm.Address, price uint64) {

}

func (ma *MockAccounting) Credit(peer swarm.Address, price uint64) error {
	return nil
}

func (ma *MockAccounting) Debit(peer swarm.Address, price uint64) error {
	return nil
}

func (ma *MockAccounting) Balance(peer swarm.Address) (int64, error) {
	return 0, nil
}

func NewAccounting() *MockAccounting {
	return &MockAccounting{}
}
