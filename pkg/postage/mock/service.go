// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"errors"
	"math/big"

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
	m := &mockPostage{}
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
	return optionFunc(func(m *mockPostage) { m.i = s })
}

type mockPostage struct {
	i         *postage.StampIssuer
	acceptAll bool
}

func (m *mockPostage) Add(s *postage.StampIssuer) {
	m.i = s
}

func (m *mockPostage) StampIssuers() []*postage.StampIssuer {
	return []*postage.StampIssuer{m.i}
}

func (m *mockPostage) GetStampIssuer(id []byte) (*postage.StampIssuer, error) {
	if m.acceptAll {
		return postage.NewStampIssuer("test fallback", "test identity", id, big.NewInt(3), 24, 6, 1000, true), nil
	}

	if m.i != nil {
		return m.i, nil
	}

	return nil, errors.New("stampissuer not found")
}

func (m *mockPostage) IssuerUsable(_ *postage.StampIssuer) bool {
	return true
}

// BatchExists returns always true.
func (m *mockPostage) BatchExists(_ []byte) (bool, error) {
	return true, nil
}

func (m *mockPostage) Handle(_ *postage.Batch) {}

func (m *mockPostage) Close() error {
	return nil
}
