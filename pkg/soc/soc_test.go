// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package swarm contains most basic and general Swarm concepts.
package soc_test

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/swarm"
	bmtlegacy "github.com/ethersphere/bmt/legacy"
)

// TestCreateChunk verifies that the chunk create from the update object
// corresponds to the soc spec.
func TestCreateChunk(t *testing.T) {
	id := make([]byte, 32)
	payload := make([]byte, 42)
	u := soc.NewUpdate(id, payload)
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)
	err = u.AddSigner(signer)
	if err != nil {
		t.Fatal(err)
	}

	ch, err := u.CreateChunk()
	if err != nil {
		t.Fatal(err)
	}
	chunkData := ch.Data()

	cursor := 0
	if !bytes.Equal(chunkData[cursor:cursor+soc.IdSize], id) {
		t.Fatal("id mismatch")
	}
	cursor += soc.IdSize
	signature := chunkData[cursor : cursor+soc.SignatureSize]
	cursor += soc.SignatureSize + swarm.SpanSize
	if !bytes.Equal(chunkData[cursor:], payload) {
		t.Fatal("payload mismatch")
	}

	publicKey, err := signer.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	ethereumAddress, err := crypto.NewEthereumAddress(*publicKey)
	if err != nil {
		t.Fatal(err)
	}

	bmtPool := bmtlegacy.NewTreePool(swarm.NewHasher, swarm.Branches, bmtlegacy.PoolSize)
	bmtHasher := bmtlegacy.New(bmtPool)
	err = bmtHasher.SetSpan(int64(len(payload)))
	if err != nil {
		t.Fatal(err)

	}
	_, err = bmtHasher.Write(payload)
	if err != nil {
		t.Fatal(err)
	}
	payloadSum := bmtHasher.Sum(nil)

	recoveredPublicKey, err := crypto.Recover(signature, payloadSum)
	if err != nil {
		t.Fatal(err)
	}
	recoveredEthereumAddress, err := crypto.NewEthereumAddress(*recoveredPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(recoveredEthereumAddress, ethereumAddress) {
		t.Fatalf("address mismatch %x %x", recoveredEthereumAddress, ethereumAddress)
	}
}

// TestUpdateFromChunk verifies that valid chunk data deserializes to
// a fully populated Update object.
func TestUpdateFromChunk(t *testing.T) {
	id := make([]byte, 32)
	payload := make([]byte, 42)
	u := soc.NewUpdate(id, payload)
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)
	err = u.AddSigner(signer)
	if err != nil {
		t.Fatal(err)
	}

	ch, err := u.CreateChunk()
	if err != nil {
		t.Fatal(err)
	}

	publicKey, err := signer.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	u2, err := soc.UpdateFromChunk(ch)
	if err != nil {
		t.Fatal(err)
	}

	ownerEthereumAddress, err := crypto.NewEthereumAddress(*publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ownerEthereumAddress, u2.OwnerAddress()) {
		t.Fatalf("owner address mismatch %x %x", ownerEthereumAddress, u2.OwnerAddress())
	}
}
