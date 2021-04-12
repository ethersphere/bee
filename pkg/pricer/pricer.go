// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pricer

import (
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/pricer/headerutils"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

var _ Interface = (*Pricer)(nil)

var gPoPrice uint64

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
	// Price based router
	CheapestPeer(addr swarm.Address, skipPeers []swarm.Address, allowUpstream bool) (swarm.Address, error)
}

type pricingPeer struct {
	lock       sync.Mutex
	address    swarm.Address
	priceTable []uint64
}

type Pricer struct {
	pricingPeersMu sync.Mutex
	pricingPeers   map[string]*pricingPeer
	peerSuggester  topology.EachPeerer
	lock           sync.Mutex
	logger         logging.Logger
	store          storage.StateStorer
	overlay        swarm.Address
	topology       topology.Driver
	poPrice        uint64
	priceTable     []uint64
}

func New(logger logging.Logger, store storage.StateStorer, overlay swarm.Address, poPrice uint64) *Pricer {
	gPoPrice = poPrice
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
	s.lock.Lock()
	defer s.lock.Unlock()
	if len(s.priceTable) > 0 {
		prt := make([]uint64, len(s.priceTable))
		copy(prt, s.priceTable)
		return prt
	}
	return s.defaultPriceTable()
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
	peerproximity := swarm.Proximity(peer.Bytes(), chunk.Bytes())

	s.lock.Lock()
	defer s.lock.Unlock()

	if len(s.priceTable) == 0 {
		if proximity >= neighborhoodDepth {
			proximity = neighborhoodDepth
			if peerproximity >= neighborhoodDepth {
				return 0, proximity
			}
		}
		return s.defaultPrice(proximity), proximity
	}

	if int(proximity) >= len(s.priceTable) {
		proximity = uint8(len(s.priceTable) - 1)
	}

	if proximity >= neighborhoodDepth {
		proximity = neighborhoodDepth
		if peerproximity >= neighborhoodDepth {
			return 0, proximity
		}
	}

	return s.priceTable[proximity], proximity
}

// pricePO returns the price for a PO from the table stored for the node.
func (s *Pricer) pricePO(po uint8) (uint64, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if len(s.priceTable) == 0 {
		return s.defaultPrice(po), nil
	}

	proximity := po
	if int(po) >= len(s.priceTable) {
		proximity = uint8(len(s.priceTable) - 1)
	}

	return s.priceTable[proximity], nil
}

// PeerPrice returns the price for the PO of a chunk from the table stored for the given peer.
// Taking into consideration that the peer might be an in-neighborhood peer,
// if the chunk is at least neighborhood depth proximate to both the node and the peer, the price is 0
func (s *Pricer) PeerPrice(peer, chunk swarm.Address) uint64 {

	neighborhoodDepth := s.neighborhoodDepth()
	proximity := swarm.Proximity(s.overlay.Bytes(), chunk.Bytes())
	peerproximity := swarm.Proximity(peer.Bytes(), chunk.Bytes())

	pricingPeer, err := s.getPricingPeer(peer)
	if err != nil {
		if peerproximity >= neighborhoodDepth {
			peerproximity = neighborhoodDepth
			if proximity >= neighborhoodDepth {
				return 0
			}
		}
		return s.defaultPrice(peerproximity)
	}

	pricingPeer.lock.Lock()
	defer pricingPeer.lock.Unlock()

	if len(pricingPeer.priceTable) == 0 {
		if peerproximity >= neighborhoodDepth {
			peerproximity = neighborhoodDepth
			if proximity >= neighborhoodDepth {
				return 0
			}
		}
		return s.defaultPrice(peerproximity)
	}

	// Determine neighborhood depth presumed by peer based on pricetable rows
	peerNeighborhoodDepth := uint8(len(pricingPeer.priceTable) - 1)

	// determine whether the chunk is within presumed neighborhood depth of peer
	if peerproximity >= peerNeighborhoodDepth {
		peerproximity = peerNeighborhoodDepth
		// determine if the chunk is within presumed neighborhood depth of peer to us
		if proximity >= peerNeighborhoodDepth {
			return 0
		}
	}

	price := pricingPeer.priceTable[peerproximity]

	return price
}

func (s *Pricer) peerPricePO(peer swarm.Address, po uint8) (uint64, error) {
	pricingPeer, err := s.getPricingPeer(peer)
	if err != nil {
		return s.defaultPrice(po), nil
	}

	pricingPeer.lock.Lock()
	defer pricingPeer.lock.Unlock()

	if int(po) >= len(pricingPeer.priceTable) {
		po = uint8(len(pricingPeer.priceTable) - 1)
	}

	return pricingPeer.priceTable[po], nil

}

func (s *Pricer) getPricingPeer(peer swarm.Address) (*pricingPeer, error) {
	s.pricingPeersMu.Lock()
	defer s.pricingPeersMu.Unlock()

	peerData, ok := s.pricingPeers[peer.String()]
	if !ok {
		peerData = &pricingPeer{address: peer}
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

	return pricingPeer.setPriceTable(priceTable)
}

func (s *Pricer) NotifyPeerPrice(peer swarm.Address, price uint64, index uint8) error {

	if price == 0 {
		return nil
	}

	pricingPeer, err := s.getPricingPeer(peer)
	if err != nil {
		return err
	}

	return pricingPeer.setPrice(index, price)
}

func (s *Pricer) defaultPriceTable() []uint64 {
	neighborhoodDepth := s.neighborhoodDepth()
	priceTable := make([]uint64, neighborhoodDepth+1)
	for i := uint8(0); i <= neighborhoodDepth; i++ {
		priceTable[i] = uint64(neighborhoodDepth-i+1) * s.poPrice
	}

	return priceTable
}

func (s *Pricer) defaultPrice(po uint8) uint64 {
	neighborhoodDepth := s.neighborhoodDepth()
	if po > neighborhoodDepth {
		po = neighborhoodDepth
	}
	return uint64(neighborhoodDepth-po+1) * s.poPrice
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

func (s *Pricer) CheapestPeer(addr swarm.Address, skipPeers []swarm.Address, allowUpstream bool) (swarm.Address, error) {
	cheapest := swarm.Address{}
	var cheapestPrice uint64
	neighborhoodDepth := s.neighborhoodDepth()
	chunkProximity := swarm.Proximity(s.overlay.Bytes(), addr.Bytes())

	err := s.peerSuggester.EachPeerRev(func(peer swarm.Address, po uint8) (bool, bool, error) {
		// If the chunk is not within neighborhood depth, we want to select from specific PO bin
		if chunkProximity < neighborhoodDepth {
			if po != chunkProximity {
				return false, false, nil
			}
		} else {
			// If the chunk is within neighborhood depth, we want to select from any bin with PO >= ND
			if po < neighborhoodDepth {
				return false, false, nil
			}
		}

		for _, a := range skipPeers {
			if a.Equal(peer) {
				return false, false, nil
			}
		}
		if cheapest.IsZero() {
			cheapest = peer
			cheapestPrice = s.PeerPrice(peer, addr)
			return false, false, nil
		}
		currentPeerPrice := s.PeerPrice(peer, addr)
		if currentPeerPrice < cheapestPrice {
			cheapest = peer
			cheapestPrice = currentPeerPrice
			return false, false, nil
		}
		if currentPeerPrice == cheapestPrice {
			dcmp, err := swarm.DistanceCmp(addr.Bytes(), cheapest.Bytes(), peer.Bytes())
			if err != nil {
				return false, false, err
			}
			switch dcmp {
			case 0:
				// do nothing
			case -1:
				// current peer is closer
				cheapest = peer
			case 1:
				// closest is already closer to chunk
				// do nothing
			}
		}

		return false, false, nil
	})
	if err != nil {
		return swarm.Address{}, err
	}

	// check if found
	if cheapest.IsZero() {
		return swarm.Address{}, topology.ErrWantSelf
	}

	if allowUpstream {
		return cheapest, nil
	}

	dcmp, err := swarm.DistanceCmp(addr.Bytes(), cheapest.Bytes(), s.overlay.Bytes())
	if err != nil {
		return swarm.Address{}, fmt.Errorf("distance compare addr %s cheapest %s base address %s: %w", addr.String(), cheapest.String(), s.overlay.String(), err)
	}
	if dcmp != 1 {
		return swarm.Address{}, topology.ErrWantSelf
	}

	return cheapest, nil
}

func (s *Pricer) SetTopology(top topology.Driver) {
	s.topology = top
	s.peerSuggester = top
}

func (p *pricingPeer) setPrice(index uint8, price uint64) error {

	var priceTable []uint64

	p.lock.Lock()
	defer p.lock.Unlock()

	if len(p.priceTable) > 0 {
		priceTable = p.priceTable
	}

	currentIndexDepth := uint8(len(priceTable)) - 1

	// Simple case, already have index depth, single value change
	if index <= currentIndexDepth {
		// if value not updated, return
		if priceTable[index] == price {
			return nil
		}
		priceTable[index] = price
		p.priceTable = priceTable
		return nil
	}

	// Complicated case, index is larger than depth of already known table
	newPriceTable := make([]uint64, index+1)

	// Copy previous content
	_ = copy(newPriceTable, priceTable)

	// Check how many rows are missing
	numberOfMissingRows := index - currentIndexDepth

	// make descending values for the missing rows, ending with newly learned value
	for i := uint8(0); i < numberOfMissingRows; i++ {
		currentrow := index - i
		newPriceTable[currentrow] = price + uint64(i)*gPoPrice
	}

	p.priceTable = newPriceTable
	return nil
}

func (p *pricingPeer) setPriceTable(priceTable []uint64) error {
	p.lock.Lock()
	p.priceTable = priceTable
	p.lock.Unlock()
	return nil
}
