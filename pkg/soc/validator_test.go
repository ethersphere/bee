// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package swarm contains most basic and general Swarm concepts.
package soc_test

import (
	"encoding/binary"
	"testing"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestSocValidator verifies that the validator can detect both
// valid soc chunks, as well as chunks with invalid data and invalid
// address.
func TestSocValidator(t *testing.T) {
	id := make([]byte, soc.IdSize)
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)

	bmtHashOfFoo := "2387e8e7d8a48c2a9339c97c1dc3461a9a7aa07e994c5cb8b38fd7c1b3e6ea48"
	address := swarm.MustParseHexAddress(bmtHashOfFoo)
	foo := "foo"
	fooLength := len(foo)
	fooBytes := make([]byte, 8+fooLength)
	binary.LittleEndian.PutUint64(fooBytes, uint64(fooLength))
	copy(fooBytes[8:], foo)
	ch := swarm.NewChunk(address, fooBytes)

	sch, err := soc.NewChunk(id, ch, signer)
	if err != nil {
		t.Fatal(err)
	}

	// check valid chunk
	v := soc.NewValidator()
	if !v.Validate(sch) {
		t.Fatal("valid chunk evaluates to invalid")
	}

	// check invalid data
	sch.Data()[0] = 0x01
	if v.Validate(sch) {
		t.Fatal("chunk with invalid data evaluates to valid")
	}

	// check invalid address
	sch.Data()[0] = 0x00
	wrongAddressBytes := sch.Address().Bytes()
	wrongAddressBytes[0] ^= wrongAddressBytes[0]
	wrongAddress := swarm.NewAddress(wrongAddressBytes)
	sch = swarm.NewChunk(wrongAddress, sch.Data())
	if v.Validate(sch) {
		t.Fatal("chunk with invalid address evaluates to valid")
	}
}
