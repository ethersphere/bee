// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"github.com/ethersphere/bee/pkg/swarm"
)

type Service struct {
	peerPrice             uint64
	price                 uint64
	peerPriceFunc         func(peer, chunk swarm.Address) uint64
	priceForPeerFunc      func(peer, chunk swarm.Address) uint64
	priceTableFunc        func() (priceTable []uint64)
	peerPriceTableFunc    func(peer, chunk swarm.Address) (priceTable []uint64, err error)
	pricePOFunc           func(PO uint8) (uint64, error)
	peerPricePOFunc       func(peer swarm.Address, PO uint8) (uint64, error)
	notifyPriceTableFunc  func(peer swarm.Address, priceTable []uint64) error
	defaultPriceTableFunc func() (priceTable []uint64)
	defaultPriceFunc      func(PO uint8) uint64
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

// WithPriceTableFunc sets the mock Release function
func WithPriceTableFunc(f func() (priceTable []uint64)) Option {
	return optionFunc(func(s *Service) {
		s.priceTableFunc = f
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
func (pricer *Service) PeerPriceTable(peer, chunk swarm.Address) (priceTable []uint64, err error) {
	if pricer.peerPriceTableFunc != nil {
		return pricer.peerPriceTableFunc(peer, chunk)
	}
	return nil, nil
}
func (pricer *Service) PricePO(PO uint8) (uint64, error) {
	if pricer.pricePOFunc != nil {
		return pricer.pricePOFunc(PO)
	}
	return 0, nil
}
func (pricer *Service) PeerPricePO(peer swarm.Address, PO uint8) (uint64, error) {
	if pricer.peerPricePOFunc != nil {
		return pricer.peerPricePOFunc(peer, PO)
	}
	return 0, nil
}
func (pricer *Service) NotifyPriceTable(peer swarm.Address, priceTable []uint64) error {
	if pricer.notifyPriceTableFunc != nil {
		return pricer.notifyPriceTableFunc(peer, priceTable)
	}
	return nil
}
func (pricer *Service) DefaultPriceTable() (priceTable []uint64) {
	if pricer.defaultPriceTableFunc != nil {
		return pricer.defaultPriceTableFunc()
	}
	return nil
}
func (pricer *Service) DefaultPrice(PO uint8) uint64 {
	if pricer.defaultPriceFunc != nil {
		return pricer.defaultPriceFunc(PO)
	}
	return 0
}

// Option is the option passed to the mock accounting service
type Option interface {
	apply(*Service)
}

type optionFunc func(*Service)

func (f optionFunc) apply(r *Service) { f(r) }
