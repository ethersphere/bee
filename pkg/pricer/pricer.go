// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pricer

import (
	"math/big"

	"github.com/ethersphere/bee/pkg/swarm"
)

// Pricer returns pricing information for chunk hashes.
type Interface interface {
	// PeerPrice is the price the peer charges for a given chunk hash.
	PeerPrice(peer, chunk swarm.Address) uint64
	// Price is the price we charge for a given chunk hash.
	Price(chunk swarm.Address) uint64
}

// FixedPricer is a Pricer that has a fixed price for chunks.
type FixedPricer struct {
	overlay swarm.Address
	poPrice uint64
}

// NewFixedPricer returns a new FixedPricer with a given price.
func NewFixedPricer(overlay swarm.Address, poPrice uint64) *FixedPricer {
	return &FixedPricer{
		overlay: overlay,
		poPrice: poPrice,
	}
}

// PeerPrice implements Pricer.
func (pricer *FixedPricer) PeerPrice(peer, chunk swarm.Address) uint64 {
	return uint64(swarm.MaxPO-swarm.Proximity(peer.Bytes(), chunk.Bytes())+1) * pricer.poPrice
}

// Price implements Pricer.
func (pricer *FixedPricer) Price(chunk swarm.Address) uint64 {
	return pricer.PeerPrice(pricer.overlay, chunk)
}

func (pricer *FixedPricer) MostExpensive() *big.Int {
	poPrice := new(big.Int).SetUint64(pricer.poPrice)
	maxPO := new(big.Int).SetUint64(uint64(swarm.MaxPO))
	tenTimesMaxPO := new(big.Int).Mul(big.NewInt(10), maxPO)
	return new(big.Int).Mul(tenTimesMaxPO, poPrice)
}
