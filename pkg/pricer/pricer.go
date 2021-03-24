// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pricer

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/pricer/headerutils"
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
	// PriceTable returns pricetable stored for the node
	PriceTable() []uint64
	// PeerPrice is the price the peer charges for a given chunk hash.
	PeerPrice(peer, chunk swarm.Address) uint64
	// PriceForPeer is the price we charge a peer for a given chunk hash.
	PriceForPeer(peer, chunk swarm.Address) uint64
	// NotifyPriceTable saves a provided pricetable for a peer to store
	NotifyPriceTable(peer swarm.Address, priceTable []uint64) error
	// NotifyPeerPrice changes a value that belongs to an index in a peer pricetable
	NotifyPeerPrice(peer swarm.Address, price uint64, index uint8) error
	// PriceHeadler creates response headers with pricing information
	PriceHeadler(p2p.Headers, swarm.Address) p2p.Headers
}

var (
	ErrPersistingBalancePeer = errors.New("failed to persist pricetable for peer")
)

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

func New(logger logging.Logger, store storage.StateStorer, overlay swarm.Address, poPrice uint64) *Pricer {
	return &Pricer{
		logger:       logger,
		pricingPeers: make(map[string]*pricingPeer),
		store:        store,
		overlay:      overlay,
		poPrice:      poPrice,
	}
}

// PriceTable returns the pricetable stored for the node
// If not available, the default pricetable is provided
func (s *Pricer) PriceTable() (priceTable []uint64) {
	err := s.store.Get(priceTableKey(), &priceTable)
	if err != nil {
		priceTable = s.defaultPriceTable()
	}
	return priceTable
}

// peerPriceTable returns the price table stored for the given peer.
// If we can't get price table from store, we return the default price table
func (s *Pricer) peerPriceTable(peer swarm.Address) (priceTable []uint64) {
	err := s.store.Get(peerPriceTableKey(peer), &priceTable)
	if err != nil {
		priceTable = s.defaultPriceTable() // get default pricetable
	}
	return priceTable
}

// PriceForPeer returns the price for the PO of a chunk from the table stored for the node.
// Taking into consideration that the peer might be an in-neighborhood peer,
// if the chunk is at least neighborhood depth proximate to both the node and the peer, the price is 0
func (s *Pricer) PriceForPeer(peer, chunk swarm.Address) uint64 {
	proximity := swarm.Proximity(s.overlay.Bytes(), chunk.Bytes())
	neighborhoodDepth := s.neighborhoodDepth()

	if proximity >= neighborhoodDepth {
		peerproximity := swarm.Proximity(peer.Bytes(), chunk.Bytes())
		if peerproximity >= neighborhoodDepth {
			return 0
		}
	}

	price, err := s.pricePO(proximity)

	if err != nil {
		price = s.defaultPrice(proximity)
	}

	return price
}

// priceWithIndexForPeer returns price for a chunk for a given peer,
// and the index of PO in pricetable which is used
func (s *Pricer) priceWithIndexForPeer(peer, chunk swarm.Address) (price uint64, index uint8) {
	proximity := swarm.Proximity(s.overlay.Bytes(), chunk.Bytes())
	neighborhoodDepth := s.neighborhoodDepth()

	priceTable := s.PriceTable()

	if int(proximity) >= len(priceTable) {
		proximity = uint8(len(priceTable) - 1)
	}

	if proximity >= neighborhoodDepth {
		proximity = neighborhoodDepth
		peerproximity := swarm.Proximity(peer.Bytes(), chunk.Bytes())
		if peerproximity >= neighborhoodDepth {
			return 0, proximity
		}
	}

	return priceTable[proximity], proximity
}

// pricePO returns the price for a PO from the table stored for the node.
func (s *Pricer) pricePO(PO uint8) (uint64, error) {
	priceTable := s.PriceTable()

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
		peerNeighborhoodDepth = s.neighborhoodDepth()
	}

	// determine whether the chunk is within presumed neighborhood depth of peer
	if proximity >= peerNeighborhoodDepth {
		// determine if the chunk is within presumed neighborhood depth of peer to us
		selfproximity := swarm.Proximity(s.overlay.Bytes(), chunk.Bytes())
		if selfproximity >= peerNeighborhoodDepth {
			return 0
		}
	}

	price, err := s.peerPricePO(peer, proximity)

	if err != nil {
		price = s.defaultPrice(proximity)
	}

	return price
}

// peerPricePO returns the price for a PO from the table stored for the given peer.
func (s *Pricer) peerPricePO(peer swarm.Address, PO uint8) (uint64, error) {
	var priceTable []uint64
	err := s.store.Get(peerPriceTableKey(peer), &priceTable)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return 0, err
		}
		priceTable = s.defaultPriceTable()
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

func (s *Pricer) storePriceTable(peer swarm.Address, priceTable []uint64) error {
	s.logger.Tracef("Storing pricetable %v for peer %v", priceTable, peer)
	err := s.store.Put(peerPriceTableKey(peer), priceTable)
	if err != nil {
		return err
	}
	return nil
}

// NotifyPriceTable should be called to notify pricer of changes in the peers pricetable
func (s *Pricer) NotifyPriceTable(peer swarm.Address, priceTable []uint64) error {
	pricingPeer, err := s.getPricingPeer(peer)
	if err != nil {
		return err
	}

	pricingPeer.lock.Lock()
	defer pricingPeer.lock.Unlock()

	return s.storePriceTable(peer, priceTable)
}

func (s *Pricer) NotifyPeerPrice(peer swarm.Address, price uint64, index uint8) error {

	if price == 0 {
		return nil
	}

	pricingPeer, err := s.getPricingPeer(peer)
	if err != nil {
		return err
	}

	pricingPeer.lock.Lock()
	defer pricingPeer.lock.Unlock()

	priceTable := s.peerPriceTable(peer)
	currentIndexDepth := uint8(len(priceTable)) - 1

	if index <= currentIndexDepth {
		// Simple case, already have index depth, single value change
		priceTable[index] = price
		return s.storePriceTable(peer, priceTable)
	}

	// Complicated case, index is larger than depth of already known table
	newPriceTable := make([]uint64, index+1)

	// Copy previous content
	_ = copy(newPriceTable, priceTable)

	// Check how many rows are missing
	numberOfMissingRows := index - currentIndexDepth

	for i := uint8(0); i < numberOfMissingRows; i++ {
		currentrow := index - i
		newPriceTable[currentrow] = price + uint64(i)*s.poPrice
		s.logger.Debugf("Guessing price %v for extending pricetable %v for peer %v", newPriceTable[currentrow], newPriceTable, peer)
	}

	return s.storePriceTable(peer, newPriceTable)
}

func (s *Pricer) defaultPriceTable() []uint64 {
	neighborhoodDepth := s.neighborhoodDepth()
	priceTable := make([]uint64, neighborhoodDepth+1)
	for i := uint8(0); i <= neighborhoodDepth; i++ {
		priceTable[i] = uint64(neighborhoodDepth-i+1) * s.poPrice
	}

	return priceTable
}

func (s *Pricer) defaultPrice(PO uint8) uint64 {
	neighborhoodDepth := s.neighborhoodDepth()
	if PO > neighborhoodDepth {
		PO = neighborhoodDepth
	}
	return uint64(neighborhoodDepth-PO+1) * s.poPrice
}

func (s *Pricer) neighborhoodDepth() uint8 {
	var neighborhoodDepth uint8
	if s.topology != nil {
		neighborhoodDepth = s.topology.NeighborhoodDepth()
	}
	return neighborhoodDepth
}

func (s *Pricer) PriceHeadler(receivedHeaders p2p.Headers, peerAddress swarm.Address) (returnHeaders p2p.Headers) {

	chunkAddress, receivedPrice, err := headerutils.ParsePricingHeaders(receivedHeaders)
	if err != nil {
		return p2p.Headers{
			"error": []byte("Error reading pricing headers"),
		}
	}

	s.logger.Debugf("price headler: received target %v with price as %v, from peer %s", chunkAddress, receivedPrice, peerAddress)
	checkPrice, index := s.priceWithIndexForPeer(peerAddress, chunkAddress)

	returnHeaders, err = headerutils.MakePricingResponseHeaders(checkPrice, chunkAddress, index)
	if err != nil {
		return p2p.Headers{
			"error": []byte("Error creating response pricing headers"),
		}
	}
	s.logger.Debugf("price headler: response target %v with price as %v, for peer %s", chunkAddress, checkPrice, peerAddress)

	return returnHeaders

}

func (s *Pricer) SetTopology(top topology.Driver) {
	s.topology = top
}
