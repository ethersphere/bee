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
	addressDataPair map[string][]byte // Make validator accept more than one address/data pair
}

// NewMockValidator constructs a new MockValidator
func NewMockValidator(address swarm.Address, data []byte) *MockValidator {
	mp := &MockValidator{
		addressDataPair: make(map[string][]byte),
	}
	mp.addressDataPair[address.String()] = data
	return mp
}

// Add a new address/data pair which can be validated
func (v *MockValidator) AddPair(address swarm.Address, data []byte) {
	v.addressDataPair[address.String()] = data
}

// Validate checks the passed chunk for validity
func (v *MockValidator) Validate(ch swarm.Chunk) (valid bool) {
	if data, ok := v.addressDataPair[ch.Address().String()]; ok {
		if bytes.Equal(data, ch.Data()) {
			return true
		}
	}
	return false
}
