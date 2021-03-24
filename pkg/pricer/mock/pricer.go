// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
)

type Service struct {
	peerPrice            uint64
	price                uint64
	peerPriceFunc        func(peer, chunk swarm.Address) uint64
	priceForPeerFunc     func(peer, chunk swarm.Address) uint64
	priceTableFunc       func() (priceTable []uint64)
	notifyPriceTableFunc func(peer swarm.Address, priceTable []uint64) error
	priceHeadlerFunc     func(p2p.Headers, swarm.Address) p2p.Headers
	notifyPeerPriceFunc  func(peer swarm.Address, price uint64, index uint8) error
}

// WithReserveFunc sets the mock Reserve function
func WithPeerPriceFunc(f func(peer, chunk swarm.Address) uint64) Option {
	return optionFunc(func(s *Service) {
		s.peerPriceFunc = f
	})
}

// WithReleaseFunc sets the mock Release function
func WithPriceForPeerFunc(f func(peer, chunk swarm.Address) uint64) Option {
	return optionFunc(func(s *Service) {
		s.priceForPeerFunc = f
	})
}

func WithPrice(p uint64) Option {
	return optionFunc(func(s *Service) {
		s.price = p
	})
}

func WithPeerPrice(p uint64) Option {
	return optionFunc(func(s *Service) {
		s.peerPrice = p
	})
}

// WithPriceTableFunc sets the mock Release function
func WithPriceTableFunc(f func() (priceTable []uint64)) Option {
	return optionFunc(func(s *Service) {
		s.priceTableFunc = f
	})
}

func WithPriceHeadlerFunc(f func(headers p2p.Headers, addr swarm.Address) p2p.Headers) Option {
	return optionFunc(func(s *Service) {
		s.priceHeadlerFunc = f
	})
}

func NewMockService(opts ...Option) *Service {
	mock := new(Service)
	mock.price = 10
	mock.peerPrice = 10
	for _, o := range opts {
		o.apply(mock)
	}
	return mock
}

func (pricer *Service) PeerPrice(peer, chunk swarm.Address) uint64 {
	if pricer.peerPriceFunc != nil {
		return pricer.peerPriceFunc(peer, chunk)
	}
	return pricer.peerPrice
}

func (pricer *Service) PriceForPeer(peer, chunk swarm.Address) uint64 {
	if pricer.priceForPeerFunc != nil {
		return pricer.priceForPeerFunc(peer, chunk)
	}
	return pricer.price
}

func (pricer *Service) PriceTable() (priceTable []uint64) {
	if pricer.priceTableFunc != nil {
		return pricer.priceTableFunc()
	}
	return nil
}
func (pricer *Service) NotifyPriceTable(peer swarm.Address, priceTable []uint64) error {
	if pricer.notifyPriceTableFunc != nil {
		return pricer.notifyPriceTableFunc(peer, priceTable)
	}
	return nil
}

func (pricer *Service) PriceHeadler(headers p2p.Headers, addr swarm.Address) p2p.Headers {
	if pricer.priceHeadlerFunc != nil {
		return pricer.priceHeadlerFunc(headers, addr)
	}
	return p2p.Headers{}
}

func (pricer *Service) NotifyPeerPrice(peer swarm.Address, price uint64, index uint8) error {
	if pricer.notifyPeerPriceFunc != nil {
		return pricer.notifyPeerPriceFunc(peer, price, index)
	}
	return nil
}

// Option is the option passed to the mock accounting service
type Option interface {
	apply(*Service)
}

type optionFunc func(*Service)

func (f optionFunc) apply(r *Service) { f(r) }
