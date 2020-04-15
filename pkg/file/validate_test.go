// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package file_test

import (
	"encoding/binary"
	"testing"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestContentAddressValidator checks that the validator evaluates correctly
// on valid and invalid input
func TestContentAddressValidator(t *testing.T) {

	// instantiate validator
	validator := file.NewContentAddressValidator()

	// generate address from pre-generated hex of 'foo' from legacy bmt
	bmtHashOfFoo := "b9d678ef39fa973b430795a1f04e0f2541b47c996fd300552a1e8bfb5824325f"
	address := swarm.MustParseHexAddress(bmtHashOfFoo)

	// set up a chunk object with correct expected length prefix
	// and test validation result
	foo := "foo"
	fooLength := len(foo)
	fooBytes := make([]byte, 8+fooLength)
	binary.LittleEndian.PutUint64(fooBytes, uint64(fooLength))
	copy(fooBytes[8:], []byte(foo))
	ch := swarm.NewChunk(address, fooBytes)
	if !validator.Validate(ch) {
		t.Fatalf("data '%s' should have validated to hash '%x'", ch.Data(), ch.Address())
	}

	// now test with incorrect data
	ch = swarm.NewChunk(address, fooBytes[:len(fooBytes)-1])
	if validator.Validate(ch) {
		t.Fatalf("data '%s' should not have validated to hash '%x'", ch.Data(), ch.Address())
	}
}
