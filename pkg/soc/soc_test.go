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
	u := soc.NewChunk(id, payload)
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

	// verify that id, signature, payload is in place
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

	// get the public key of the signer that was used
	publicKey, err := signer.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	ethereumAddress, err := crypto.NewEthereumAddress(*publicKey)
	if err != nil {
		t.Fatal(err)
	}

	// calculate the sum of the payload and use it to recover the owner
	// from the chunk data
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

	sha3Hasher := swarm.NewHasher()
	_, err = sha3Hasher.Write(payloadSum)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sha3Hasher.Write(id)
	if err != nil {
		t.Fatal(err)
	}
	toSignBytes := sha3Hasher.Sum(nil)

	// verify owner match
	recoveredPublicKey, err := crypto.Recover(signature, toSignBytes)
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

// TestFromChunk verifies that valid chunk data deserializes to
// a fully populated Chunk object.
func TestFromChunk(t *testing.T) {
	id := make([]byte, 32)
	payload := make([]byte, 42)
	u := soc.NewChunk(id, payload)
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)
	err = u.AddSigner(signer)
	if err != nil {
		t.Fatal(err)
	}

	// CreateChunk has already been verified in previous test
	ch, err := u.CreateChunk()
	if err != nil {
		t.Fatal(err)
	}

	u2, err := soc.FromChunk(ch)
	if err != nil {
		t.Fatal(err)
	}

	// owner matching means the address was successfully recovered from
	// payload and signature
	publicKey, err := signer.PublicKey()
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
