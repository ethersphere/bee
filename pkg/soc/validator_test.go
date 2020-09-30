// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package swarm contains most basic and general Swarm concepts.
package soc_test

import (
	"crypto/rand"
	"encoding/binary"
	"io"
	"testing"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestValidator verifies that the validator can detect both
// valid soc chunks, as well as chunks with invalid data and invalid
// address.
func TestValidator(t *testing.T) {
	id := make([]byte, soc.IdSize)
	_, err := io.ReadFull(rand.Reader, id)
	if err != nil {
		t.Fatal(err)
	}
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

	t.Run("valid chunk", func(t *testing.T) {
		sch, err := soc.NewChunk(id, ch, signer)
		if err != nil {
			t.Fatal(err)
		}
		if !soc.Valid(sch) {
			t.Fatal("valid chunk evaluates to invalid")
		}
	})

	t.Run("invalid chunk (content addressed)", func(t *testing.T) {
		if soc.Valid(ch) {
			t.Fatal("invalid chunk evaluates to valid")
		}
	})
	t.Run("invalid chunk", func(t *testing.T) {
		sch, err := soc.NewChunk(id, ch, signer)
		if err != nil {
			t.Fatal(err)
		}
		sch.Data()[0] = 0x00
		if soc.Valid(sch) {
			t.Fatal("invalid chunk evaluates to valid")
		}
	})
	t.Run("invalid chunk", func(t *testing.T) {
		sch, err := soc.NewChunk(id, ch, signer)
		if err != nil {
			t.Fatal(err)
		}
		addr := sch.Address().Bytes()
		addr[0] = 0x00
		sch = swarm.NewChunk(swarm.NewAddress(addr), sch.Data())
		if soc.Valid(sch) {
			t.Fatal("invalid chunk evaluates to valid")
		}
	})
}
