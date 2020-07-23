// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package swarm contains most basic and general Swarm concepts.
package soc

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/content"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	IdSize        = 32
	SignatureSize = 65
	AddressSize   = crypto.AddressSize
	minChunkSize  = IdSize + SignatureSize + swarm.SpanSize
)

// Id is a soc identifier
type Id []byte

// Owner is a wrapper that enforces valid length address of soc owner.
type Owner struct {
	address []byte
}

// NewOwner creates a new Owner.
func NewOwner(address []byte) (*Owner, error) {
	if len(address) != AddressSize {
		return nil, fmt.Errorf("invalid address %x", address)
	}
	return &Owner{
		address: address,
	}, nil
}

// Soc wraps a single soc.
type Soc struct {
	id        Id
	signature []byte
	signer    crypto.Signer
	owner     *Owner
	Chunk     swarm.Chunk
}

// NewChunk is a convenience function to create a single-owner chunk ready to be sent
// on the network.
func NewChunk(id Id, ch swarm.Chunk, signer crypto.Signer) (swarm.Chunk, error) {
	s := New(id, ch)
	err := s.AddSigner(signer)
	if err != nil {
		return nil, err
	}
	return s.ToChunk()
}

// New creates a new Soc representation from arbitrary soc id and
// a content-addressed chunk.
//
// By default the span of the soc data is set to the length
// of the payload.
func New(id Id, ch swarm.Chunk) *Soc {
	return &Soc{
		id:    id,
		Chunk: ch,
	}
}

// WithOwnerAddress provides the possibility of setting the ethereum
// address for the owner of an soc in the absence of a signer.
func (s *Soc) WithOwnerAddress(ownerAddress *Owner) *Soc {
	s.owner = ownerAddress
	return s
}

// AddSigner currently sets a single signer for the soc.
//
// This method will overwrite any value set with WithOwnerAddress with
// the address derived from the given signer.
func (s *Soc) AddSigner(signer crypto.Signer) error {
	publicKey, err := signer.PublicKey()
	if err != nil {
		return err
	}
	ownerAddressBytes, err := crypto.NewEthereumAddress(*publicKey)
	if err != nil {
		return err
	}
	ownerAddress, err := NewOwner(ownerAddressBytes)
	if err != nil {
		return err
	}
	s.signer = signer
	s.owner = ownerAddress
	return nil
}

// OwnerAddress returns the ethereum address of the signer of the Chunk.
func (s *Soc) OwnerAddress() []byte {
	return s.owner.address
}

// Address returns the soc Chunk address.
func (s *Soc) Address() (swarm.Address, error) {
	return CreateAddress(s.id, s.owner)
}

// FromChunk recreates an Soc representation from swarm.Chunk data.
func FromChunk(sch swarm.Chunk) (*Soc, error) {
	chunkData := sch.Data()
	if len(chunkData) < minChunkSize {
		return nil, errors.New("less than minimum length")
	}

	// add all the data fields
	s := &Soc{}
	cursor := 0

	s.id = chunkData[cursor : cursor+IdSize]
	cursor += IdSize

	s.signature = chunkData[cursor : cursor+SignatureSize]
	cursor += SignatureSize

	spanBytes := chunkData[cursor : cursor+swarm.SpanSize]
	cursor += swarm.SpanSize

	ch, err := content.NewChunkWithSpanBytes(chunkData[cursor:], spanBytes)
	if err != nil {
		return nil, err
	}

	toSignBytes, err := toSignDigest(s.id, ch.Address().Bytes())
	if err != nil {
		return nil, err
	}

	// recover owner information
	recoveredEthereumAddress, err := recoverAddress(s.signature, toSignBytes)
	if err != nil {
		return nil, err
	}
	owner, err := NewOwner(recoveredEthereumAddress)
	if err != nil {
		return nil, err
	}
	s.owner = owner
	s.Chunk = ch

	return s, nil
}

// ToChunk generates a signed chunk payload ready for submission to the swarm network.
//
// The method will fail if no signer has been added.
func (s *Soc) ToChunk() (swarm.Chunk, error) {
	var err error
	if s.signer == nil {
		return nil, errors.New("signer missing")
	}

	// generate the data to sign
	toSignBytes, err := toSignDigest(s.id, s.Chunk.Address().Bytes())
	if err != nil {
		return nil, err
	}

	// sign the chunk
	signature, err := s.signer.Sign(toSignBytes)
	if err != nil {
		return nil, err
	}

	// prepare the payload
	buf := bytes.NewBuffer(nil)
	buf.Write(s.id)
	buf.Write(signature)
	buf.Write(s.Chunk.Data())

	// create chunk
	socAddress, err := s.Address()
	if err != nil {
		return nil, err
	}
	return swarm.NewChunk(socAddress, buf.Bytes()), nil
}

// toSignDigest creates a digest suitable for signing to represent the soc.
func toSignDigest(id Id, sum []byte) ([]byte, error) {
	h := swarm.NewHasher()
	_, err := h.Write(id)
	if err != nil {
		return nil, err
	}
	_, err = h.Write(sum)
	if err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

// CreateAddress creates a new soc address from the soc id and the ethereum address of the signer
func CreateAddress(id Id, owner *Owner) (swarm.Address, error) {
	h := swarm.NewHasher()
	_, err := h.Write(id)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	_, err = h.Write(owner.address)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	sum := h.Sum(nil)
	return swarm.NewAddress(sum), nil
}

// recoverOwner returns the ethereum address of the owner of an soc.
func recoverAddress(signature, digest []byte) ([]byte, error) {
	recoveredPublicKey, err := crypto.Recover(signature, digest)
	if err != nil {
		return nil, err
	}
	recoveredEthereumAddress, err := crypto.NewEthereumAddress(*recoveredPublicKey)
	if err != nil {
		return nil, err
	}
	return recoveredEthereumAddress, nil
}
