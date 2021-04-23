// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"

	"github.com/ethersphere/bee/pkg/pinning"
	"github.com/ethersphere/bee/pkg/swarm"
)

var _ pinning.Interface = (*ServiceMock)(nil)

// NewServiceMock is a convenient constructor for creating ServiceMock.
func NewServiceMock() *ServiceMock {
	return &ServiceMock{index: make(map[string]int)}
}

// ServiceMock represents a simple mock of pinning.Interface.
// The implementation is not goroutine-safe.
type ServiceMock struct {
	index   map[string]int
	entries []swarm.Address
}

// CreatePin implements pinning.Interface CreatePin method.
func (sm *ServiceMock) CreatePin(_ context.Context, addr swarm.Address, _ bool) error {
	if _, ok := sm.index[addr.String()]; ok {
		return nil
	}
	sm.index[addr.String()] = len(sm.entries)
	sm.entries = append(sm.entries, addr)
	return nil
}

// DeletePin implements pinning.Interface DeletePin method.
func (sm *ServiceMock) DeletePin(_ context.Context, addr swarm.Address) error {
	i, ok := sm.index[addr.String()]
	if !ok {
		return nil
	}
	delete(sm.index, addr.String())
	sm.entries = append(sm.entries[:i], sm.entries[i+1:]...)
	return nil
}

// HasPin implements pinning.Interface HasPin method.
func (sm *ServiceMock) HasPin(addr swarm.Address) (bool, error) {
	_, ok := sm.index[addr.String()]
	return ok, nil
}

// Pins implements pinning.Interface Pins method.
func (sm *ServiceMock) Pins() ([]swarm.Address, error) {
	return append([]swarm.Address(nil), sm.entries...), nil
}

// Entries returns all pinned entries.
func (sm *ServiceMock) Entries() []swarm.Address {
	return sm.entries
}
