// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"errors"

	"github.com/ethersphere/bee/pkg/bzz"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	ma "github.com/multiformats/go-multiaddr"
)

type Service struct {
	addProtocolFunc func(p2p.ProtocolSpec) error
	connectFunc     func(ctx context.Context, addr ma.Multiaddr, notify bool) (address *bzz.Address, err error)
	disconnectFunc  func(overlay swarm.Address) error
	peersFunc       func() []p2p.Peer
	setNotifierFunc func(topology.Notifier)
	addressesFunc   func() ([]ma.Multiaddr, error)
}

func WithAddProtocolFunc(f func(p2p.ProtocolSpec) error) Option {
	return optionFunc(func(s *Service) {
		s.addProtocolFunc = f
	})
}

func WithConnectFunc(f func(ctx context.Context, addr ma.Multiaddr, notify bool) (address *bzz.Address, err error)) Option {
	return optionFunc(func(s *Service) {
		s.connectFunc = f
	})
}

func WithDisconnectFunc(f func(overlay swarm.Address) error) Option {
	return optionFunc(func(s *Service) {
		s.disconnectFunc = f
	})
}

func WithPeersFunc(f func() []p2p.Peer) Option {
	return optionFunc(func(s *Service) {
		s.peersFunc = f
	})
}

func WithSetNotifierFunc(f func(topology.Notifier)) Option {
	return optionFunc(func(s *Service) {
		s.setNotifierFunc = f
	})
}

func WithAddressesFunc(f func() ([]ma.Multiaddr, error)) Option {
	return optionFunc(func(s *Service) {
		s.addressesFunc = f
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
		return errors.New("function AddProtocol not configured")
	}
	return s.addProtocolFunc(spec)
}

func (s *Service) Connect(ctx context.Context, addr ma.Multiaddr, notify bool) (address *bzz.Address, err error) {
	if s.connectFunc == nil {
		return nil, errors.New("function Connect not configured")
	}
	return s.connectFunc(ctx, addr, notify)
}

func (s *Service) Disconnect(overlay swarm.Address) error {
	if s.disconnectFunc == nil {
		return errors.New("function Disconnect not configured")
	}
	return s.disconnectFunc(overlay)
}

func (s *Service) SetNotifier(f topology.Notifier) {
	if s.setNotifierFunc == nil {
		return
	}

	s.setNotifierFunc(f)
}

func (s *Service) Addresses() ([]ma.Multiaddr, error) {
	if s.addressesFunc == nil {
		return nil, errors.New("function Addresses not configured")
	}
	return s.addressesFunc()
}

func (s *Service) Peers() []p2p.Peer {
	if s.peersFunc == nil {
		return nil
	}
	return s.peersFunc()
}

type Option interface {
	apply(*Service)
}
type optionFunc func(*Service)

func (f optionFunc) apply(r *Service) { f(r) }
