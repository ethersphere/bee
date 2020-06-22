// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package swarm contains most basic and general Swarm concepts.
package soc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"

	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/crypto"
	bmtlegacy "github.com/ethersphere/bmt/legacy"
)

const (
	IdSize = 32
	SignatureSize = 65
)

// Id is a soc identifier
type Id []byte

// Owner is a wrapper that enforces valid length address owner for soc.
type Owner struct {
	address []byte
}

// NewOwner creates a new Owner.
func NewOwner(address []byte) (*Owner, error) {
	if len(address) != 20 {
		return nil, fmt.Errorf("invalid address %x", address)
	}
	return &Owner{
		address: address,
	}, nil
}

// Update wraps a single soc update.
type Update struct {
	id Id
	span int64
	payload []byte
	signature []byte
	signer crypto.Signer
	owner *Owner
	ch swarm.Chunk
}

// NewUpdate creates a new Update from arbitrary soc id and
// soc byte payload.
//
// By default the span of the soc data is set to the length
// of the payload.
func NewUpdate(id Id, payload []byte) *Update {
	return &Update{
		id: id,
		payload: payload,
		span: int64(len(payload)),
	}
}

// WithSpan is a chainable function that adds an arbitrary
// span to the chunk data defined for the update.
func (s *Update) WithSpan(span int64) *Update {
	s.span = span
	return s
}

// WithOwnerAddress provides the possibility of setting the ethereum
// address for the owner of an update in the absence of a signer.
func (s *Update) WithOwnerAddress(ownerAddress *Owner) *Update {
	s.owner = ownerAddress
	return s
}

// AddSigner currently sets a single signer for the soc update. 
//
// This method will overwrite any value set with WithOwnerAddress with
// the address derived from the given signer.
func (s *Update) AddSigner(signer crypto.Signer) error {
	publicKey, err := signer.PublicKey()
	if err != nil {
		return err
	}
	ownerAddressBytes, err := crypto.NewEthereumAddress(*publicKey)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "%x", ownerAddressBytes)
	ownerAddress, err := NewOwner(ownerAddressBytes)
	if err != nil {
		return err
	}
	s.signer = signer
	s.owner = ownerAddress
	return nil
}

// OwnerAddress returns the ethereum address of the signer of the Update.
func (s *Update) OwnerAddress() []byte {
	return s.owner.address
}

// Address returns the soc Chunk address of the update.
func (s *Update) Address() swarm.Address {
	hasher := swarm.NewHasher()
	hasher.Write(s.id)
	hasher.Write(s.owner.address)
	addressBytes := hasher.Sum(nil)
	return swarm.NewAddress(addressBytes)
}

// UpdateFromChunk recreates an Update from swarm.Chunk data.
func UpdateFromChunk(ch swarm.Chunk) (*Update, error) {
	chunkData := ch.Data()
	minUpdateSize := IdSize+SignatureSize+swarm.SpanSize
	if len(chunkData) < minUpdateSize {
		return nil, errors.New("less than minimum length")
	}

	update := &Update{}
	cursor := 0
	update.id = chunkData[cursor:cursor+IdSize]
	cursor += IdSize
	update.signature = chunkData[cursor:cursor+SignatureSize]
	cursor += SignatureSize
	spanBytes := chunkData[cursor:cursor+swarm.SpanSize]
	span := binary.LittleEndian.Uint64(spanBytes)
	update.span = int64(span)
	cursor += swarm.SpanSize
	update.payload = chunkData[cursor:]

	bmtPool := bmtlegacy.NewTreePool(swarm.NewHasher, swarm.Branches, bmtlegacy.PoolSize)
	bmtHasher := bmtlegacy.New(bmtPool)

	err := bmtHasher.SetSpan(int64(span))
	if err != nil {
		return nil, err
	}
	_, err = bmtHasher.Write(update.payload)
	if err != nil {
		return nil, err
	}
	payloadSum := bmtHasher.Sum(nil)

	recoveredPublicKey, err := crypto.Recover(update.signature, payloadSum)
	if err != nil {
		return nil, err
	}
	recoveredEthereumAddress, err := crypto.NewEthereumAddress(*recoveredPublicKey)
	if err != nil {
		return nil, err
	}
	owner, err := NewOwner(recoveredEthereumAddress)
	if err != nil {
		return nil, err
	}
	update.owner = owner

	return update, nil
}

// CreateChunk creates a new chunk with signed payload ready for submission to the swarm network
// from the given update data.
//
// This method will fail if no signer has been defined.
func (s *Update) CreateChunk() (swarm.Chunk, error) {
	if s.signer == nil {
		return nil, errors.New("signer missing")
	}
	bmtPool := bmtlegacy.NewTreePool(swarm.NewHasher, swarm.Branches, bmtlegacy.PoolSize)
	bmtHasher := bmtlegacy.New(bmtPool)

	err := bmtHasher.SetSpan(s.span)
	if err != nil {
		return nil, err
	}
	_, err = bmtHasher.Write(s.payload)
	if err != nil {
		return nil, err
	}
	payloadSum := bmtHasher.Sum(nil)

	signature, err := s.signer.Sign(payloadSum)
	if err != nil {
		return nil, err
	}

	publicKey, err := s.signer.PublicKey()
	if err != nil {
		return nil, err
	}
	ethereumAddress, err := crypto.NewEthereumAddress(*publicKey)
	if err != nil {
		return nil, err
	}
	sha3Hasher := swarm.NewHasher()
	sha3Hasher.Write(s.id)
	sha3Hasher.Write(ethereumAddress)
	addressBytes := sha3Hasher.Sum(nil)
	address := swarm.NewAddress(addressBytes)

	spanBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(spanBytes, uint64(s.span))
	buf := bytes.NewBuffer(nil)
	buf.Write(s.id)
	buf.Write(signature)
	buf.Write(spanBytes)
	buf.Write(s.payload)

	ch := swarm.NewChunk(address, buf.Bytes())
	return ch, nil
}
