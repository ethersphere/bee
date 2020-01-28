// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"errors"

	"github.com/ethersphere/bee/pkg/p2p"
	ma "github.com/multiformats/go-multiaddr"
)

type Service struct {
	addProtocolFunc func(p2p.ProtocolSpec) error
	connectFunc     func(ctx context.Context, addr ma.Multiaddr) (overlay string, err error)
	disconnectFunc  func(overlay string) error
}

func WithAddProtocolFunc(f func(p2p.ProtocolSpec) error) Option {
	return optionFunc(func(s *Service) {
		s.addProtocolFunc = f
	})
}

func WithConnectFunc(f func(ctx context.Context, addr ma.Multiaddr) (overlay string, err error)) Option {
	return optionFunc(func(s *Service) {
		s.connectFunc = f
	})
}

func WithDisconnectFunc(f func(overlay string) error) Option {
	return optionFunc(func(s *Service) {
		s.disconnectFunc = f
	})
}

func New(opts ...Option) *Service {
	s := new(Service)
	for _, o := range opts {
		o.apply(s)
	}
	return s
}

func (s *Service) AddProtocol(spec p2p.ProtocolSpec) error {
	if s.addProtocolFunc == nil {
		return errors.New("AddProtocol function not configured")
	}
	return s.addProtocolFunc(spec)
}

func (s *Service) Connect(ctx context.Context, addr ma.Multiaddr) (overlay string, err error) {
	if s.connectFunc == nil {
		return "", errors.New("Connect function not configured")
	}
	return s.connectFunc(ctx, addr)
}

func (s *Service) Disconnect(overlay string) error {
	if s.disconnectFunc == nil {
		return errors.New("Disconnect function not configured")
	}
	return s.disconnectFunc(overlay)
}

type Option interface {
	apply(*Service)
}
type optionFunc func(*Service)

func (f optionFunc) apply(r *Service) { f(r) }
