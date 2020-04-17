// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package validator

import (
	"bytes"

	"github.com/ethersphere/bee/pkg/swarm"
)

// MockValidator returns true if the data and address passed in the Validate method
// are a byte-wise match to the data and address passed to the constructor
type MockValidator struct {
	validAddress swarm.Address
	validContent []byte
	swarm.ChunkValidator
}

// NewMockValidator constructs a new MockValidator
func NewMockValidator(address swarm.Address, data []byte) *MockValidator {
	return &MockValidator{
		validAddress: address,
		validContent: data,
	}
}

// Validate checkes the passed chunk for validity
func (v *MockValidator) Validate(ch swarm.Chunk) (valid bool) {
	if !v.validAddress.Equal(ch.Address()) {
		return false
	}
	if !bytes.Equal(v.validContent, ch.Data()) {
		return false
	}
	return true
}
