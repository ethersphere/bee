// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pricer

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
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
	NotifyPriceTable(peer swarm.Address, priceTable []uint64) error
	NotifyPeerPrice(peer swarm.Address, price uint64, index uint8) error
	PriceHeadler(p2p.Headers, swarm.Address) p2p.Headers
	MakePricingHeaders(chunkPrice uint64, addr swarm.Address) (p2p.Headers, error)
	MakePricingResponseHeaders(chunkPrice uint64, addr swarm.Address, index uint8) (p2p.Headers, error)
	ReadPriceHeader(receivedHeaders p2p.Headers) (uint64, error)
	ReadPricingHeaders(receivedHeaders p2p.Headers) (swarm.Address, uint64, error)
	ReadPricingResponseHeaders(receivedHeaders p2p.Headers) (swarm.Address, uint64, uint8, error)
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

func New(logger logging.Logger, store storage.StateStorer, overlay swarm.Address, poPrice uint64) *Pricer {
	return &Pricer{
		logger:       logger,
		pricingPeers: make(map[string]*pricingPeer),
		store:        store,
		overlay:      overlay,
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
func (s *Pricer) PeerPriceTable(peer swarm.Address) (priceTable []uint64, err error) {
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
	neighborhoodDepth := s.neighborhoodDepth()

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

// PriceWithIndexForPeer returns price for a chunk for a given peer,
// and the
func (s *Pricer) PriceWithIndexForPeer(peer, chunk swarm.Address) (price uint64, index uint8) {
	proximity := swarm.Proximity(s.overlay.Bytes(), chunk.Bytes())
	neighborhoodDepth := s.neighborhoodDepth()

	var priceTable []uint64
	err := s.store.Get(priceTableKey(), &priceTable)
	if err != nil {
		priceTable = s.DefaultPriceTable()
	}

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
	s.logger.Debugf("Storing pricetable %v for peer %v", priceTable, peer)

	err = s.store.Put(peerPriceTableKey(peer), priceTable)
	if err != nil {
		return fmt.Errorf("failed to persist pricetable for peer %v: %w", peer, err)
	}

	return nil
}

func (s *Pricer) NotifyPeerPrice(peer swarm.Address, price uint64, index uint8) error {
	currentIndexDepth := uint8(0)

	priceTable, err := s.PeerPriceTable(peer)
	if err == nil {
		currentIndexDepth = uint8(len(priceTable)) - 1
		if index <= currentIndexDepth {
			// Simple case, already have stored pricetable, no new depth, single value change
			priceTable[index] = price
			return s.NotifyPriceTable(peer, priceTable)
		}
	}

	if index > currentIndexDepth {
		// Complicated case 1, index is larger than depth of already known table
		newPriceTable := make([]uint64, index+1)

		// if there was no error when reading pricetable from store
		if err == nil {
			for i := uint8(0); i <= currentIndexDepth; i++ {
				newPriceTable[i] = priceTable[i]
			}

			numberOfMissingRows := index - currentIndexDepth
			// if we have multiple missing rows, we have to guess something for the gaps,
			// since we only learned the price for index PO
			// for now we will add some default increase
			var currentrow uint8
			for i := uint8(0); i < numberOfMissingRows; i++ {
				currentrow = index - i
				newPriceTable[currentrow] = price + uint64(i)*s.poPrice
			}
			return s.NotifyPriceTable(peer, newPriceTable)
		}

		// Complicated case 2, no pricetable for peer existed yet
		numberOfMissingRows := index - currentIndexDepth
		// if we have multiple missing rows, we have to guess something for the gaps,
		// since we only learned the price for index PO
		// for now we will add some default increase
		var currentrow uint8
		for i := uint8(0); i < numberOfMissingRows; i++ {
			currentrow = index - i
			newPriceTable[currentrow] = price + uint64(i)*s.poPrice
		}
		return s.NotifyPriceTable(peer, newPriceTable)

	}

	s.logger.Debugf("Storing updated pricetable %v for peer %v", priceTable, peer)
	return s.NotifyPriceTable(peer, priceTable)
}

func (s *Pricer) DefaultPriceTable() []uint64 {
	neighborhoodDepth := s.neighborhoodDepth()
	priceTable := make([]uint64, neighborhoodDepth+1)
	for i := uint8(0); i <= neighborhoodDepth; i++ {
		priceTable[i] = uint64(neighborhoodDepth-i+1) * s.poPrice
	}

	return priceTable
}

func (s *Pricer) DefaultPrice(PO uint8) uint64 {
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

//
// S
//
// E
//
// P
//
// A
//
// R
//
// A
//
// T
//
// O
//
// R
//

func (s *Pricer) PriceHeadler(receivedHeaders p2p.Headers, peerAddress swarm.Address) (returnHeaders p2p.Headers) {

	chunkAddress, receivedPrice, err := s.ReadPricingHeaders(receivedHeaders)
	if err != nil {
		return p2p.Headers{
			"error": []byte("No target specified or error unmarshaling target streamheader of request"),
		}
	}

	s.logger.Debugf("price headler: received target %v with price as %v, from peer %s", chunkAddress, receivedPrice, peerAddress)
	checkPrice, index := s.PriceWithIndexForPeer(peerAddress, chunkAddress)

	returnHeaders, err = s.MakePricingResponseHeaders(checkPrice, chunkAddress, index)
	if err != nil {
		return p2p.Headers{
			"error": []byte("Error remarshaling target for response streamheader"),
		}
	}
	s.logger.Debugf("price headler: response target %v with price as %v, for peer %s", chunkAddress, checkPrice, peerAddress)

	return returnHeaders

}

func (s *Pricer) MakePricingHeaders(chunkPrice uint64, addr swarm.Address) (p2p.Headers, error) {

	chunkPriceInBytes := make([]byte, 8)

	binary.LittleEndian.PutUint64(chunkPriceInBytes, chunkPrice)
	chunkAddressInBytes, err := addr.MarshalJSON()
	if err != nil {
		return p2p.Headers{}, err
	}

	headers := p2p.Headers{
		"price":  chunkPriceInBytes,
		"target": chunkAddressInBytes,
	}

	return headers, nil

}

func (s *Pricer) MakePricingResponseHeaders(chunkPrice uint64, addr swarm.Address, index uint8) (p2p.Headers, error) {

	chunkPriceInBytes := make([]byte, 8)
	chunkIndexInBytes := make([]byte, 1)

	binary.LittleEndian.PutUint64(chunkPriceInBytes, chunkPrice)
	chunkIndexInBytes[0] = index
	chunkAddressInBytes, err := addr.MarshalJSON()

	if err != nil {
		return p2p.Headers{}, err
	}

	headers := p2p.Headers{
		"price":  chunkPriceInBytes,
		"target": chunkAddressInBytes,
		"index":  chunkIndexInBytes,
	}

	return headers, nil

}

// ReadPricingHeaders used by responder to read address and price from stream headers
// Returns an error if no target field attached or the contents of it are not readable
func (s *Pricer) ReadPricingHeaders(receivedHeaders p2p.Headers) (swarm.Address, uint64, error) {

	target, err := s.ReadTargetHeader(receivedHeaders)
	if err != nil {
		s.logger.Errorf("price headers: parsing received header target field error: %w", err)
		return swarm.ZeroAddress, 0, err
	}
	price, err := s.ReadPriceHeader(receivedHeaders)
	if err != nil {
		s.logger.Errorf("price headers: parsing received header price field error: %w", err)
		return swarm.ZeroAddress, 0, err
	}
	return target, price, nil

}

// ReadPricingResponseHeaders used by requester to read address, price and index from response headers
// Returns an error if any fields are missing or target is unreadable
func (s *Pricer) ReadPricingResponseHeaders(receivedHeaders p2p.Headers) (swarm.Address, uint64, uint8, error) {
	target, err := s.ReadTargetHeader(receivedHeaders)
	if err != nil {
		s.logger.Errorf("price headers: parsing received header target field error: %w", err)
		return swarm.ZeroAddress, 0, 0, err
	}
	price, err := s.ReadPriceHeader(receivedHeaders)
	if err != nil {
		s.logger.Errorf("price headers: parsing received header price field error: %w", err)
		return swarm.ZeroAddress, 0, 0, err
	}
	index, err := s.ReadIndexHeader(receivedHeaders)
	if err != nil {
		s.logger.Errorf("price headers: parsing received header index field error: %w", err)
		return swarm.ZeroAddress, 0, 0, err
	}

	return target, price, index, nil
}

func (s *Pricer) ReadIndexHeader(receivedHeaders p2p.Headers) (uint8, error) {
	if receivedHeaders["index"] == nil {
		return 0, fmt.Errorf("no index header")
	}

	index := uint8(receivedHeaders["index"][0])
	return index, nil
}

func (s *Pricer) ReadTargetHeader(receivedHeaders p2p.Headers) (swarm.Address, error) {
	if receivedHeaders["target"] == nil {
		return swarm.ZeroAddress, fmt.Errorf("no target header")
	}

	var target swarm.Address
	err := target.UnmarshalJSON(receivedHeaders["target"])
	if err != nil {
		return swarm.ZeroAddress, err
	}

	return target, nil
}

func (s *Pricer) ReadPriceHeader(receivedHeaders p2p.Headers) (uint64, error) {
	if receivedHeaders["price"] == nil {
		return 0, fmt.Errorf("no price header")
	}

	receivedPrice := binary.LittleEndian.Uint64(receivedHeaders["price"])
	return receivedPrice, nil
}

func (s *Pricer) SetKademlia(kad topology.Driver) {
	s.topology = kad
}
