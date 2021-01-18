// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pricer

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

const (
	priceTablePrefix string = "pricetable_"
)

var _ Interface = (*Pricer)(nil)

// Pricer returns pricing information for chunk hashes and proximity orders
type Interface interface {
	// PeerPrice is the price the peer charges for a given chunk hash.
	PeerPrice(peer, chunk swarm.Address) uint64
	// PriceForPeer is the price we charge for a given chunk hash.
	PriceForPeer(peer, chunk swarm.Address) uint64
	PriceTable() (priceTable []uint64)
}

type pricingPeer struct {
	lock sync.Mutex
}

type Pricer struct {
	pricingPeersMu sync.Mutex
	pricingPeers   map[string]*pricingPeer
	logger         logging.Logger
	store          storage.StateStorer
	overlay        swarm.Address
	topology       topology.Driver
	poPrice        uint64
}

func New(logger logging.Logger, store storage.StateStorer, topology topology.Driver, overlay swarm.Address, poPrice uint64) *Pricer {
	return &Pricer{
		logger:       logger,
		pricingPeers: make(map[string]*pricingPeer),
		store:        store,
		overlay:      overlay,
		topology:     topology,
		poPrice:      poPrice,
	}
}

// PeerPrice returns the price for the PO of a chunk from the table stored for the node.
func (s *Pricer) PriceTable() (priceTable []uint64) {
	err := s.store.Get(priceTableKey(), &priceTable)
	if err != nil {
		priceTable = s.DefaultPriceTable()
	}
	return priceTable
}

// PeerPriceTable returns the price table stored for the given peer.
// If we can't get price table from store, we return the default price table
func (s *Pricer) PeerPriceTable(peer, chunk swarm.Address) (priceTable []uint64, err error) {
	err = s.store.Get(peerPriceTableKey(peer), &priceTable)
	if err != nil {
		priceTable = s.DefaultPriceTable() // get default pricetable
	}
	return priceTable, nil
}

// PriceForPeer returns the price for the PO of a chunk from the table stored for the node.
// Taking into consideration that the peer might be an in-neighborhood peer,
// if the chunk is at least neighborhood depth proximate to both the node and the peer, the price is 0
func (s *Pricer) PriceForPeer(peer, chunk swarm.Address) uint64 {
	proximity := swarm.Proximity(s.overlay.Bytes(), chunk.Bytes())
	neighborhoodDepth := s.topology.NeighborhoodDepth()

	if proximity >= neighborhoodDepth {
		peerproximity := swarm.Proximity(peer.Bytes(), chunk.Bytes())
		if peerproximity >= neighborhoodDepth {
			return 0
		}
	}

	price, err := s.PricePO(proximity)

	if err != nil {
		price = s.DefaultPrice(proximity)
	}

	return price
}

// PricePO returns the price for a PO from the table stored for the node.
func (s *Pricer) PricePO(PO uint8) (uint64, error) {
	var priceTable []uint64
	err := s.store.Get(priceTableKey(), &priceTable)
	if err != nil {
		priceTable = s.DefaultPriceTable()
	}

	proximity := PO
	if int(PO) >= len(priceTable) {
		proximity = uint8(len(priceTable) - 1)
	}

	return priceTable[proximity], nil
}

// PeerPrice returns the price for the PO of a chunk from the table stored for the given peer.
// Taking into consideration that the peer might be an in-neighborhood peer,
// if the chunk is at least neighborhood depth proximate to both the node and the peer, the price is 0
func (s *Pricer) PeerPrice(peer, chunk swarm.Address) uint64 {
	proximity := swarm.Proximity(peer.Bytes(), chunk.Bytes())

	// Determine neighborhood depth presumed by peer based on pricetable rows
	var priceTable []uint64
	err := s.store.Get(peerPriceTableKey(peer), &priceTable)

	peerNeighborhoodDepth := uint8(len(priceTable) - 1)
	if err != nil {
		peerNeighborhoodDepth = s.topology.NeighborhoodDepth()
	}

	// determine whether the chunk is within presumed neighborhood depth of peer
	if proximity >= peerNeighborhoodDepth {
		// determine if we are as well if the chunk is within presumed neighborhood depth of peer to us
		selfproximity := swarm.Proximity(s.overlay.Bytes(), chunk.Bytes())
		if selfproximity >= peerNeighborhoodDepth {
			return 0
		}
	}

	price, err := s.PeerPricePO(peer, proximity)

	if err != nil {
		price = s.DefaultPrice(proximity)
	}

	return price
}

// PeerPricePO returns the price for a PO from the table stored for the given peer.
func (s *Pricer) PeerPricePO(peer swarm.Address, PO uint8) (uint64, error) {
	var priceTable []uint64
	err := s.store.Get(peerPriceTableKey(peer), &priceTable)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return 0, err
		}
		priceTable = s.DefaultPriceTable()
	}

	proximity := PO
	if int(PO) >= len(priceTable) {
		proximity = uint8(len(priceTable) - 1)
	}

	return priceTable[proximity], nil

}

// peerPriceTableKey returns the price table storage key for the given peer.
func peerPriceTableKey(peer swarm.Address) string {
	return fmt.Sprintf("%s%s", priceTablePrefix, peer.String())
}

// priceTableKey returns the price table storage key for own price table
func priceTableKey() string {
	return fmt.Sprintf("%s%s", priceTablePrefix, "self")
}

func (s *Pricer) getPricingPeer(peer swarm.Address) (*pricingPeer, error) {
	s.pricingPeersMu.Lock()
	defer s.pricingPeersMu.Unlock()

	peerData, ok := s.pricingPeers[peer.String()]
	if !ok {
		peerData = &pricingPeer{}
		s.pricingPeers[peer.String()] = peerData
	}

	return peerData, nil
}

// NotifyPriceTable should be called to notify pricer of changes in the peers pricetable
func (s *Pricer) NotifyPriceTable(peer swarm.Address, priceTable []uint64) error {
	pricingPeer, err := s.getPricingPeer(peer)
	if err != nil {
		return err
	}

	pricingPeer.lock.Lock()
	defer pricingPeer.lock.Unlock()

	err = s.store.Put(peerPriceTableKey(peer), priceTable)
	if err != nil {
		return fmt.Errorf("failed to persist pricetable for peer %v: %w", peer, err)
	}

	return nil
}

func (s *Pricer) DefaultPriceTable() []uint64 {
	neighborhoodDepth := s.topology.NeighborhoodDepth()
	priceTable := make([]uint64, neighborhoodDepth+1)
	s.logger.Debugf("kkkkk %v", neighborhoodDepth)
	for i := uint8(0); i <= neighborhoodDepth; i++ {
		priceTable[i] = uint64(neighborhoodDepth-i+1) * s.poPrice
	}

	return priceTable
}

func (s *Pricer) DefaultPrice(PO uint8) uint64 {
	neighborhoodDepth := s.topology.NeighborhoodDepth()
	if PO > neighborhoodDepth {
		PO = neighborhoodDepth
	}
	return uint64(neighborhoodDepth-PO+1) * s.poPrice
}
