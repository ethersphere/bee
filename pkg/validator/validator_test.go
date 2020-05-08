// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package validator_test

import (
	"encoding/binary"
	"testing"

	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/validator"
)

// TestContentAddressValidator checks that the validator evaluates correctly
// on valid and invalid input
func TestContentAddressValidator(t *testing.T) {

	// instantiate validator
	validator := validator.NewContentAddressValidator()

	// generate address from pre-generated hex of 'foo' from legacy bmt
	bmtHashOfFoo := "2387e8e7d8a48c2a9339c97c1dc3461a9a7aa07e994c5cb8b38fd7c1b3e6ea48"
	address := swarm.MustParseHexAddress(bmtHashOfFoo)

	// set up a chunk object with correct expected length prefix
	// and test validation result
	foo := "foo"
	fooLength := len(foo)
	fooBytes := make([]byte, 8+fooLength)
	binary.LittleEndian.PutUint64(fooBytes, uint64(fooLength))
	copy(fooBytes[8:], foo)
	ch := swarm.NewChunk(address, fooBytes)
	if !validator.Validate(ch) {
		t.Fatalf("data '%s' should have validated to hash '%s'", ch.Data(), ch.Address())
	}

	// now test with incorrect data
	ch = swarm.NewChunk(address, fooBytes[:len(fooBytes)-1])
	if validator.Validate(ch) {
		t.Fatalf("data '%s' should not have validated to hash '%s'", ch.Data(), ch.Address())
	}
}
