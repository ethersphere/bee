// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package soc provides the single-owner chunk implemenation
// and validator.
package soc

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	IdSize        = 32
	SignatureSize = 65
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
	if len(address) != crypto.AddressSize {
		return nil, fmt.Errorf("invalid address %x", address)
	}
	return &Owner{
		address: address,
	}, nil
}

// RawSoc wraps a single-owner chunk.
type RawSoc struct {
	id        Id
	signature []byte
	owner     *Owner
	chunk     swarm.Chunk // wrapped chunk
}

// Soc wraps a single-owner chunk and contains a signer interface to sign it.
type Soc struct {
	*RawSoc
	signer crypto.Signer
}

// NewSoc creates a single-owner chunk.
// It does not sign the chunk.
func NewSoc(id Id, ch swarm.Chunk, signer crypto.Signer) (*Soc, error) {
	publicKey, err := signer.PublicKey()
	if err != nil {
		return nil, err
	}
	ownerAddressBytes, err := crypto.NewEthereumAddress(*publicKey)
	if err != nil {
		return nil, err
	}
	ownerAddress, err := NewOwner(ownerAddressBytes)
	if err != nil {
		return nil, err
	}
	rs := &RawSoc{
		id:    id,
		owner: ownerAddress,
		chunk: ch,
	}
	return &Soc{RawSoc: rs, signer: signer}, nil
}

// NewSignedSoc creates a single-owner chunk based on already signed data.
func NewSignedSoc(id Id, ch swarm.Chunk, owner, sig []byte) (*RawSoc, error) {
	o, err := NewOwner(owner)
	if err != nil {
		return nil, err
	}

	return &RawSoc{
		id:        id,
		signature: sig,
		owner:     o,
		chunk:     ch,
	}, nil
}

// Chunk returns the soc chunk.
func (rs *RawSoc) Chunk() (swarm.Chunk, error) {
	socAddress, err := rs.Address()
	if err != nil {
		return nil, err
	}
	return swarm.NewChunk(socAddress, rs.toBytes()), nil
}

// OwnerAddress returns the ethereum address of the signer of the Chunk.
func (rs *RawSoc) OwnerAddress() []byte {
	return rs.owner.address
}

// Address returns the soc chunk address.
func (rs *RawSoc) Address() (swarm.Address, error) {
	return CreateAddress(rs.id, rs.owner)
}

// Signature returns the soc signature.
func (rs *RawSoc) Signature() []byte {
	return rs.signature
}

// WrappedChunk returns the chunk wrapped by the soc.
func (rs *RawSoc) WrappedChunk() swarm.Chunk {
	return rs.chunk
}

// toBytes is a helper function to convert the RawSoc data to bytes.
func (rs *RawSoc) toBytes() []byte {
	buf := bytes.NewBuffer(nil)
	buf.Write(rs.id)
	buf.Write(rs.signature)
	buf.Write(rs.chunk.Data())
	return buf.Bytes()
}

// Sign signs a soc using the signer.
// It returns a signed chunk ready for submission to the network.
func (s *Soc) Sign() (swarm.Chunk, error) {
	// generate the data to sign
	toSignBytes, err := toSignDigest(s.id, s.chunk.Address().Bytes())
	if err != nil {
		return nil, err
	}

	// sign the chunk
	signature, err := s.signer.Sign(toSignBytes)
	if err != nil {
		return nil, err
	}
	s.signature = signature

	// create chunk
	socAddress, err := s.Address()
	if err != nil {
		return nil, err
	}
	return swarm.NewChunk(socAddress, s.toBytes()), nil
}

// FromChunk recreates a RawSoc representation from swarm.Chunk data.
func FromChunk(sch swarm.Chunk) (*RawSoc, error) {
	chunkData := sch.Data()
	if len(chunkData) < minChunkSize {
		return nil, errors.New("less than minimum length")
	}

	// add all the data fields
	s := &RawSoc{}
	cursor := 0

	s.id = chunkData[cursor : cursor+IdSize]
	cursor += IdSize

	s.signature = chunkData[cursor : cursor+SignatureSize]
	cursor += SignatureSize

	ch, err := cac.NewWithDataSpan(chunkData[cursor:])
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
	s.chunk = ch

	return s, nil
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

// CreateAddress creates a new soc address from the soc id and the ethereum address of the signer.
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

// recoverAddress returns the ethereum address of the owner of an soc.
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
