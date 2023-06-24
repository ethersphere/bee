// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"bytes"
	"math/big"
	"sync"

	"github.com/ethersphere/bee/pkg/postage"
)

type optionFunc func(*mockPostage)

// Option is an option passed to a mock postage Service.
type Option interface {
	apply(*mockPostage)
}

func (f optionFunc) apply(r *mockPostage) { f(r) }

// New creates a new mock postage service.
func New(o ...Option) postage.Service {
	m := &mockPostage{
		issuersMap:   make(map[string]*postage.StampIssuer),
		issuersInUse: make(map[string]*postage.StampIssuer),
	}
	for _, v := range o {
		v.apply(m)
	}

	return m
}

// WithAcceptAll sets the mock to return a new BatchIssuer on every
// call to GetStampIssuer.
func WithAcceptAll() Option {
	return optionFunc(func(m *mockPostage) { m.acceptAll = true })
}

func WithIssuer(s *postage.StampIssuer) Option {
	return optionFunc(func(m *mockPostage) {
		m.issuersMap = map[string]*postage.StampIssuer{string(s.ID()): s}
	})
}

type mockPostage struct {
	issuersMap   map[string]*postage.StampIssuer
	issuerLock   sync.Mutex
	acceptAll    bool
	issuersInUse map[string]*postage.StampIssuer
}

func (m *mockPostage) SetExpired() error {
	return nil
}

func (m *mockPostage) HandleStampExpiry(id []byte) error {
	m.issuerLock.Lock()
	defer m.issuerLock.Unlock()

	for _, v := range m.issuersMap {
		if bytes.Equal(id, v.ID()) {
			v.SetExpired(true)
		}
	}
	return nil
}

func (m *mockPostage) Add(s *postage.StampIssuer) error {
	m.issuerLock.Lock()
	defer m.issuerLock.Unlock()

	m.issuersMap[string(s.ID())] = s
	return nil
}

func (m *mockPostage) StampIssuers() ([]*postage.StampIssuer, error) {
	m.issuerLock.Lock()
	defer m.issuerLock.Unlock()

	issuers := []*postage.StampIssuer{}
	for _, v := range m.issuersMap {
		issuers = append(issuers, v)
	}
	return issuers, nil
}

func (m *mockPostage) GetStampIssuer(id []byte) (*postage.StampIssuer, func(bool) error, error) {
	if m.acceptAll {
		return postage.NewStampIssuer("test fallback", "test identity", id, big.NewInt(3), 24, 6, 1000, true), func(_ bool) error { return nil }, nil
	}

	m.issuerLock.Lock()
	defer m.issuerLock.Unlock()

	i, exists := m.issuersMap[string(id)]
	if !exists {
		return nil, nil, postage.ErrNotFound
	}

	if _, inUse := m.issuersInUse[string(id)]; inUse {
		return nil, nil, postage.ErrBatchInUse
	}
	m.issuersInUse[string(id)] = i
	return i, func(_ bool) error {
		m.issuerLock.Lock()
		defer m.issuerLock.Unlock()
		delete(m.issuersInUse, string(id))
		return nil
	}, nil
}

func (m *mockPostage) IssuerUsable(_ *postage.StampIssuer) bool {
	return true
}

func (m *mockPostage) HandleCreate(_ *postage.Batch, _ *big.Int) error { return nil }

func (m *mockPostage) HandleTopUp(_ []byte, _ *big.Int) error { return nil }

func (m *mockPostage) HandleDepthIncrease(_ []byte, _ uint8) error { return nil }

func (m *mockPostage) Close() error {
	return nil
}
