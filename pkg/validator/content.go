// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package validator contains file-oriented chunk validation implementations
package validator

import (
	"encoding/binary"

	"github.com/ethersphere/bee/pkg/swarm"
	bmtlegacy "github.com/ethersphere/bmt/legacy"
)

var _ swarm.ChunkValidator = (*ContentAddressValidator)(nil)

// ContentAddressValidator validates that the address of a given chunk
// is the content address of its contents.
type ContentAddressValidator struct {
}

// New constructs a new ContentAddressValidator
func NewContentAddressValidator() swarm.ChunkValidator {

	return &ContentAddressValidator{}
}

// Validate performs the validation check.
func (v *ContentAddressValidator) Validate(ch swarm.Chunk) (valid bool) {
	p := bmtlegacy.NewTreePool(swarm.NewHasher, swarm.Branches, bmtlegacy.PoolSize)
	hasher := bmtlegacy.New(p)

	// prepare data
	data := ch.Data()
	address := ch.Address()
	span := binary.LittleEndian.Uint64(data[:8])

	// execute hash, compare and return result
	hasher.Reset()
	err := hasher.SetSpan(int64(span))
	if err != nil {
		return false
	}
	_, err = hasher.Write(data[8:])
	if err != nil {
		return false
	}
	s := hasher.Sum(nil)

	return address.Equal(swarm.NewAddress(s))
}
