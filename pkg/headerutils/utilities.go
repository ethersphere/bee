// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headerutils

import (
	"encoding/binary"
	"fmt"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Headers, utility functions

func MakePricingHeaders(chunkPrice uint64, addr swarm.Address) (p2p.Headers, error) {

	chunkPriceInBytes := make([]byte, 8)

	binary.LittleEndian.PutUint64(chunkPriceInBytes, chunkPrice)

	headers := p2p.Headers{
		"price":  chunkPriceInBytes,
		"target": addr.Bytes(),
	}

	return headers, nil

}

func MakePricingResponseHeaders(chunkPrice uint64, addr swarm.Address, index uint8) (p2p.Headers, error) {

	chunkPriceInBytes := make([]byte, 8)
	chunkIndexInBytes := make([]byte, 1)

	binary.LittleEndian.PutUint64(chunkPriceInBytes, chunkPrice)
	chunkIndexInBytes[0] = index

	headers := p2p.Headers{
		"price":  chunkPriceInBytes,
		"target": addr.Bytes(),
		"index":  chunkIndexInBytes,
	}

	return headers, nil

}

// ReadPricingHeaders used by responder to read address and price from stream headers
// Returns an error if no target field attached or the contents of it are not readable
func ReadPricingHeaders(receivedHeaders p2p.Headers) (swarm.Address, uint64, error) {

	target, err := ReadTargetHeader(receivedHeaders)
	if err != nil {
		return swarm.ZeroAddress, 0, err
	}
	price, err := ReadPriceHeader(receivedHeaders)
	if err != nil {
		return swarm.ZeroAddress, 0, err
	}
	return target, price, nil

}

// ReadPricingResponseHeaders used by requester to read address, price and index from response headers
// Returns an error if any fields are missing or target is unreadable
func ReadPricingResponseHeaders(receivedHeaders p2p.Headers) (swarm.Address, uint64, uint8, error) {
	target, err := ReadTargetHeader(receivedHeaders)
	if err != nil {
		return swarm.ZeroAddress, 0, 0, err
	}
	price, err := ReadPriceHeader(receivedHeaders)
	if err != nil {
		return swarm.ZeroAddress, 0, 0, err
	}
	index, err := ReadIndexHeader(receivedHeaders)
	if err != nil {
		return swarm.ZeroAddress, 0, 0, err
	}

	return target, price, index, nil
}

func ReadIndexHeader(receivedHeaders p2p.Headers) (uint8, error) {
	if receivedHeaders["index"] == nil {
		return 0, fmt.Errorf("no index header")
	}

	index := receivedHeaders["index"][0]
	return index, nil
}

func ReadTargetHeader(receivedHeaders p2p.Headers) (swarm.Address, error) {
	if receivedHeaders["target"] == nil {
		return swarm.ZeroAddress, fmt.Errorf("no target header")
	}

	var target swarm.Address
	target = swarm.NewAddress(receivedHeaders["target"])

	return target, nil
}

func ReadPriceHeader(receivedHeaders p2p.Headers) (uint64, error) {
	if receivedHeaders["price"] == nil {
		return 0, fmt.Errorf("no price header")
	}

	receivedPrice := binary.LittleEndian.Uint64(receivedHeaders["price"])
	return receivedPrice, nil
}
