// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package validator contains file-oriented chunk validation implementations
package validator

import (
	"encoding/binary"
	"hash"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bmt"
	bmtlegacy "github.com/ethersphere/bmt/legacy"
	"golang.org/x/crypto/sha3"
)

var _ swarm.ChunkValidator = (*ContentAddressValidator)(nil)

func hashFunc() hash.Hash {
	return sha3.NewLegacyKeccak256()
}

// ContentAddressValidator validates that the address of a given chunk
// is the content address of its contents
type ContentAddressValidator struct {
	hasher bmt.Hash
	logger logging.Logger
}

// New constructs a new ContentAddressValidator
func NewContentAddressValidator() swarm.ChunkValidator {
	p := bmtlegacy.NewTreePool(hashFunc, swarm.Branches, bmtlegacy.PoolSize)

	return &ContentAddressValidator{
		hasher: bmtlegacy.New(p),
	}
}

// Validate performs the validation check
func (v *ContentAddressValidator) Validate(ch swarm.Chunk) (valid bool) {

	// prepare data
	data := ch.Data()
	address := ch.Address()
	span := binary.LittleEndian.Uint64(data[:8])

	// execute hash, compare and return result
	v.hasher.Reset()
	err := v.hasher.SetSpan(int64(span))
	if err != nil {
		v.logger.Debugf("SetSpan on bmt legacy hasher gave error: %v", err)
		return false
	}
	_, err = v.hasher.Write(data[8:])
	if err != nil {
		v.logger.Debugf("Write on bmt legacy hasher gave error: %v", err)
		return false
	}
	s := v.hasher.Sum(nil)

	return address.Equal(swarm.NewAddress(s))
}
