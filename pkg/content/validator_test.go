// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package content_test

import (
	"encoding/binary"
	"testing"

	"github.com/ethersphere/bee/pkg/content"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestValidator checks that the validator evaluates correctly
// on valid and invalid input
func TestValidator(t *testing.T) {

	// instantiate validator
	validator := content.NewValidator()

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
	yes, _ := validator.Validate(ch)
	if !yes {
		t.Fatalf("data '%s' should have validated to hash '%s'", ch.Data(), ch.Address())
	}

	// now test with incorrect data
	ch = swarm.NewChunk(address, fooBytes[:len(fooBytes)-1])
	yes, cType := validator.Validate(ch)
	if yes && cType == swarm.ContentChunk {
		t.Fatalf("data '%s' should not have validated to hash '%s'", ch.Data(), ch.Address())
	}
}
