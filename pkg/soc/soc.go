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

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/swarm"
	bmtlegacy "github.com/ethersphere/bmt/legacy"
)

const (
	IdSize        = 32
	SignatureSize = 65
	AddressSize   = crypto.AddressSize
	minChunkSize  = IdSize + SignatureSize + swarm.SpanSize
)

var (
	bmtPool = bmtlegacy.NewTreePool(swarm.NewHasher, swarm.Branches, bmtlegacy.PoolSize)
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
	chunk     swarm.Chunk
}

// NewSoc creates a new Soc from arbitrary soc id and
// a content-addressed chunk.
//
// By default the span of the soc data is set to the length
// of the payload.
func NewSoc(id Id, ch swarm.Chunk) *Soc {
	return &Soc{
		id:    id,
		chunk: ch,
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

// FromChunk recreates an Chunk from swarm.Chunk data.
func FromChunk(ch swarm.Chunk) (*Soc, error) {
	chunkData := ch.Data()
	if len(chunkData) < minChunkSize {
		return nil, errors.New("less than minimum length")
	}

	// add all the data fields
	sch := &Soc{}
	cursor := 0

	sch.id = chunkData[cursor : cursor+IdSize]
	cursor += IdSize

	sch.signature = chunkData[cursor : cursor+SignatureSize]
	cursor += SignatureSize

	span := binary.LittleEndian.Uint64(chunkData[cursor : cursor+swarm.SpanSize])

	bmtHasher := bmtlegacy.New(bmtPool)
	err := bmtHasher.SetSpan(int64(span))
	chunkWithSpanData := chunkData[cursor:]
	if err != nil {
		return nil, err
	}
	cursor += swarm.SpanSize
	_, err = bmtHasher.Write(chunkData[cursor:])
	if err != nil {
		return nil, err
	}
	bmtSum := bmtHasher.Sum(nil)
	address := swarm.NewAddress(bmtSum)

	h := swarm.NewHasher()
	_, err = h.Write(sch.id)
	if err != nil {
		return nil, err
	}
	_, err = h.Write(bmtSum)
	if err != nil {
		return nil, err
	}
	toSignBytes := h.Sum(nil)

	// recover owner information
	recoveredPublicKey, err := crypto.Recover(sch.signature, toSignBytes)
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
	sch.owner = owner
	sch.chunk = swarm.NewChunk(address, chunkWithSpanData)

	return sch, nil
}

// CreateChunk creates a new chunk with signed payload ready for submission to the swarm network
// from the given chunk data.
//
// This method will fail if no signer has been defined.
func (s *Soc) ToChunk() (swarm.Chunk, error) {
	var err error
	if s.signer == nil {
		return nil, errors.New("signer missing")
	}

	h := swarm.NewHasher()
	_, err = h.Write(s.id)
	if err != nil {
		return nil, err
	}
	_, err = h.Write(s.chunk.Address().Bytes())
	if err != nil {
		return nil, err
	}
	toSignBytes := h.Sum(nil)

	// sign the chunk
	signature, err := s.signer.Sign(toSignBytes)
	if err != nil {
		return nil, err
	}

	// prepare the payload
	buf := bytes.NewBuffer(nil)
	buf.Write(s.id)
	buf.Write(signature)
	buf.Write(s.chunk.Data())

	// create chunk
	socAddress, err := s.Address()
	if err != nil {
		return nil, err
	}
	ch := swarm.NewChunk(socAddress, buf.Bytes())
	return ch, nil
}

// CreateAddress creates a new soc address from the soc id and the ethereum address of the signer
func CreateAddress(id Id, owner *Owner) (swarm.Address, error) {
	sha3Hasher := swarm.NewHasher()
	_, err := sha3Hasher.Write(id)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	_, err = sha3Hasher.Write(owner.address)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	sum := sha3Hasher.Sum(nil)
	return swarm.NewAddress(sum), nil
}
