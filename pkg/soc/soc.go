// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package soc provides the single-owner chunk implementation
// and validator.
package soc

import (
	"bytes"
	"errors"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	IdSize        = 32
	SignatureSize = 65
	minChunkSize  = IdSize + SignatureSize + swarm.SpanSize
)

var (
	errInvalidAddress = errors.New("soc: invalid address")
	errWrongChunkSize = errors.New("soc: chunk length is less than minimum")
)

// ID is a soc identifier
type ID []byte

// Soc wraps a content-addressed chunk.
type Soc struct {
	id        ID
	owner     []byte // Owner is the address in bytes of soc owner.
	signature []byte
	chunk     swarm.Chunk // wrapped chunk.
}

// New creates a new Soc representation from arbitrary soc id and
// a content-addressed chunk.
func New(id ID, ch swarm.Chunk) *Soc {
	return &Soc{
		id:    id,
		chunk: ch,
	}
}

// NewSignedSoc creates a single-owner chunk based on already signed data.
func NewSigned(id ID, ch swarm.Chunk, owner, sig []byte) (*Soc, error) {
	s := New(id, ch)
	if len(owner) != crypto.AddressSize {
		return nil, errInvalidAddress
	}
	s.owner = owner
	s.signature = sig
	return s, nil
}

// address returns the soc chunk address.
func (s *Soc) address() (swarm.Address, error) {
	if len(s.owner) != crypto.AddressSize {
		return swarm.ZeroAddress, errInvalidAddress
	}
	return CreateAddress(s.id, s.owner)
}

// WrappedChunk returns the chunk wrapped by the soc.
func (s *Soc) WrappedChunk() swarm.Chunk {
	return s.chunk
}

// Chunk returns the soc chunk.
func (s *Soc) Chunk() (swarm.Chunk, error) {
	socAddress, err := s.address()
	if err != nil {
		return nil, err
	}
	return swarm.NewChunk(socAddress, s.toBytes()), nil
}

// toBytes is a helper function to convert the RawSoc data to bytes.
func (s *Soc) toBytes() []byte {
	buf := bytes.NewBuffer(nil)
	buf.Write(s.id)
	buf.Write(s.signature)
	buf.Write(s.chunk.Data())
	return buf.Bytes()
}

// Sign signs a soc using the signer.
// It returns a signed chunk ready for submission to the network.
func (s *Soc) Sign(signer crypto.Signer) (swarm.Chunk, error) {
	// create owner
	publicKey, err := signer.PublicKey()
	if err != nil {
		return nil, err
	}
	ownerAddressBytes, err := crypto.NewEthereumAddress(*publicKey)
	if err != nil {
		return nil, err
	}
	if len(ownerAddressBytes) != crypto.AddressSize {
		return nil, errInvalidAddress
	}
	s.owner = ownerAddressBytes

	// generate the data to sign
	toSignBytes, err := hash(s.id, s.chunk.Address().Bytes())
	if err != nil {
		return nil, err
	}

	// sign the chunk
	signature, err := signer.Sign(toSignBytes)
	if err != nil {
		return nil, err
	}
	s.signature = signature

	return s.Chunk()
}

// FromChunk recreates a Soc representation from swarm.Chunk data.
func FromChunk(sch swarm.Chunk) (*Soc, error) {
	chunkData := sch.Data()
	if len(chunkData) < minChunkSize {
		return nil, errWrongChunkSize
	}

	// add all the data fields
	s := &Soc{}
	cursor := 0

	s.id = chunkData[cursor:IdSize]
	cursor += IdSize

	s.signature = chunkData[cursor : cursor+SignatureSize]
	cursor += SignatureSize

	ch, err := cac.NewWithDataSpan(chunkData[cursor:])
	if err != nil {
		return nil, err
	}

	toSignBytes, err := hash(s.id, ch.Address().Bytes())
	if err != nil {
		return nil, err
	}

	// recover owner information
	recoveredOwnerAddress, err := recoverAddress(s.signature, toSignBytes)
	if err != nil {
		return nil, err
	}
	if len(recoveredOwnerAddress) != crypto.AddressSize {
		return nil, errInvalidAddress
	}
	s.owner = recoveredOwnerAddress
	s.chunk = ch

	return s, nil
}

// CreateAddress creates a new soc address from the soc id and the ethereum address of the signer.
func CreateAddress(id ID, owner []byte) (swarm.Address, error) {
	sum, err := hash(id, owner)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	return swarm.NewAddress(sum), nil
}

// hash hashes the given values in order.
func hash(values ...[]byte) ([]byte, error) {
	h := swarm.NewHasher()
	for _, v := range values {
		_, err := h.Write(v)
		if err != nil {
			return nil, err
		}
	}
	return h.Sum(nil), nil
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
