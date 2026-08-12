// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package mock provides a mock BPS service for testing downstream consumers.
package mock

import (
	"context"
	"errors"

	"github.com/ethersphere/bee/v2/pkg/bps"
	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// ErrNotImplemented is returned by a call the test did not configure.
var ErrNotImplemented = errors.New("bps mock: not implemented")

type (
	openFunc      func(context.Context, swarm.Address, *pb.CohortSpec, *pb.PublisherAuth) (bps.Publisher, error)
	subscribeFunc func(context.Context, swarm.Address, swarm.Address, *pb.PublisherAuth) (bps.Publisher, error)
)

// Service is a mock BPS service.
type Service struct {
	open      openFunc
	subscribe subscribeFunc
}

// Option configures the mock.
type Option interface {
	apply(*Service)
}

type optionFunc func(*Service)

func (f optionFunc) apply(s *Service) { f(s) }

// WithOpenFunc sets the function called by Open.
func WithOpenFunc(f openFunc) Option {
	return optionFunc(func(s *Service) { s.open = f })
}

// WithSubscribeFunc sets the function called by Subscribe.
func WithSubscribeFunc(f subscribeFunc) Option {
	return optionFunc(func(s *Service) { s.subscribe = f })
}

// New returns a new mock service.
func New(opts ...Option) *Service {
	s := new(Service)
	for _, o := range opts {
		o.apply(s)
	}
	return s
}

// Open calls the configured open function.
func (s *Service) Open(ctx context.Context, peer swarm.Address, spec *pb.CohortSpec, auth *pb.PublisherAuth) (bps.Publisher, error) {
	if s.open == nil {
		return nil, ErrNotImplemented
	}
	return s.open(ctx, peer, spec, auth)
}

// Subscribe calls the configured subscribe function.
func (s *Service) Subscribe(ctx context.Context, peer swarm.Address, topic swarm.Address, auth *pb.PublisherAuth) (bps.Publisher, error) {
	if s.subscribe == nil {
		return nil, ErrNotImplemented
	}
	return s.subscribe(ctx, peer, topic, auth)
}
