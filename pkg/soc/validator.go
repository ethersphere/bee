// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package soc

import (
	"github.com/ethersphere/bee/pkg/swarm"
)

var _ swarm.Validator = (*SocValidator)(nil)

// SocVaildator validates that the address of a given chunk
// is a single-owner chunk.
type SocValidator struct {
}

// NewSocValidator creates a new SocValidator.
func NewSocValidator() swarm.Validator {
	return &SocValidator{}
}

// Validate performs the validation check.
func (v *SocValidator) Validate(ch swarm.Chunk) (valid bool) {
	s, err := FromChunk(ch)
	if err != nil {
		return false
	}

	address, err := s.Address()
	if err != nil {
		return false
	}
	return ch.Address().Equal(address)
}
