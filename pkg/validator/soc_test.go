// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package swarm contains most basic and general Swarm concepts.
package validator_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/validator"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/crypto"
)

// TestSocValidator verifies that a given chunk is a valid soc chunk.
func TestSocValidator(t *testing.T) {
	id := make([]byte, soc.IdSize)
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)

	payload := make([]byte, 42)
	u := soc.NewUpdate(id, payload)
	err = u.AddSigner(signer)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := u.CreateChunk()
	if err != nil {
		t.Fatal(err)
	}

	v := validator.NewSocValidator()
	if !v.Validate(ch) {
		t.Fatal("valid chunk evaluates to invalid")
	}
	ch.Data()[0] = 0x01
	if v.Validate(ch) {
		t.Fatal("chunk with invalid data evaluates to valid")
	}
	ch.Data()[0] = 0x00
	wrongAddressBytes := ch.Address().Bytes()
	// replace with not
	wrongAddressBytes[0] ^= wrongAddressBytes[0]
	wrongAddress := swarm.NewAddress(wrongAddressBytes)
	ch = swarm.NewChunk(wrongAddress, ch.Data())
	if v.Validate(ch) {
		t.Fatal("chunk with invalid data evaluates to valid")
	}
}
