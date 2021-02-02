// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headerutils

import (
	"encoding/binary"
	"errors"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	// ErrFieldLenth denotes p2p.Header having malformed field length in bytes
	ErrFieldLenth = errors.New("field length error")
	// ErrNoIndexHeader denotes p2p.Header lacking specified field
	ErrNoIndexHeader = errors.New("no index header")
	// ErrNoTargetHeader denotes p2p.Header lacking specified field
	ErrNoTargetHeader = errors.New("no target header")
	// ErrNoPriceHeader denotes p2p.Header lacking specified field
	ErrNoPriceHeader = errors.New("no price header")
)

// Headers, utility functions

func MakePricingHeaders(chunkPrice uint64, addr swarm.Address) (p2p.Headers, error) {

	chunkPriceInBytes := make([]byte, 8)

	binary.BigEndian.PutUint64(chunkPriceInBytes, chunkPrice)

	headers := p2p.Headers{
		"price":  chunkPriceInBytes,
		"target": addr.Bytes(),
	}

	return headers, nil
}

func MakePricingResponseHeaders(chunkPrice uint64, addr swarm.Address, index uint8) (p2p.Headers, error) {

	chunkPriceInBytes := make([]byte, 8)
	chunkIndexInBytes := make([]byte, 1)

	binary.BigEndian.PutUint64(chunkPriceInBytes, chunkPrice)
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
		return 0, ErrNoIndexHeader
	}

	if len(receivedHeaders["index"]) != 1 {
		return 0, ErrFieldLenth
	}

	index := receivedHeaders["index"][0]
	return index, nil
}

func ReadTargetHeader(receivedHeaders p2p.Headers) (swarm.Address, error) {
	if receivedHeaders["target"] == nil {
		return swarm.ZeroAddress, ErrNoTargetHeader
	}

	target := swarm.NewAddress(receivedHeaders["target"])

	return target, nil
}

func ReadPriceHeader(receivedHeaders p2p.Headers) (uint64, error) {
	if receivedHeaders["price"] == nil {
		return 0, ErrNoPriceHeader
	}

	if len(receivedHeaders["price"]) != 8 {
		return 0, ErrFieldLenth
	}

	receivedPrice := binary.BigEndian.Uint64(receivedHeaders["price"])
	return receivedPrice, nil
}
