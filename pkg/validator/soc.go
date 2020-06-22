// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package validator contains file-oriented chunk validation implementations
package validator

import (
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/swarm"
)

var _ swarm.ChunkValidator = (*SocValidator)(nil)

// SocVaildator validates that the address of a given chunk
// is a single-owner chunk.
type SocValidator struct {
}

// NewSocValidator creates a new SocValidator.
func NewSocValidator() swarm.ChunkValidator {
	return &SocValidator{}
}

// Validate performs the validation check.
func (v *SocValidator) Validate(ch swarm.Chunk) (valid bool) {
	update, err := soc.UpdateFromChunk(ch)
	if err != nil {
		return false
	}

	address, err := update.Address()
	if err != nil {
		return false
	}
	return ch.Address().Equal(address)
}
