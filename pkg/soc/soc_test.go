// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package swarm contains most basic and general Swarm concepts.
package soc_test

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"testing"

	"github.com/ethersphere/bee/pkg/content"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestToChunk verifies that the chunk create from the Soc object
// corresponds to the soc spec.
func TestToChunk(t *testing.T) {
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)

	id := make([]byte, 32)

	payload := []byte("foo")
	ch, err := content.NewChunk(payload)
	if err != nil {
		t.Fatal(err)
	}
	sch, err := soc.NewChunk(id, ch, signer)
	if err != nil {
		t.Fatal(err)
	}
	chunkData := sch.Data()

	// verify that id, signature, payload is in place
	cursor := 0
	if !bytes.Equal(chunkData[cursor:cursor+soc.IdSize], id) {
		t.Fatal("id mismatch")
	}
	cursor += soc.IdSize
	signature := chunkData[cursor : cursor+soc.SignatureSize]
	cursor += soc.SignatureSize

	spanBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(spanBytes, uint64(len(payload)))
	if !bytes.Equal(chunkData[cursor:cursor+swarm.SpanSize], spanBytes) {
		t.Fatal("span mismatch")
	}

	cursor += swarm.SpanSize
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

	toSignBytes, err := soc.ToSignDigest(id, ch.Address().Bytes())
	if err != nil {
		t.Fatal(err)
	}

	// verify owner match
	recoveredEthereumAddress, err := soc.RecoverAddress(signature, toSignBytes)
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
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)

	id := make([]byte, 32)

	payload := make([]byte, swarm.ChunkSize)
	_, err = rand.Read(payload)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := content.NewChunk(payload)
	if err != nil {
		t.Fatal(err)
	}
	sch, err := soc.NewChunk(id, ch, signer)
	if err != nil {
		t.Fatal(err)
	}

	u2, err := soc.FromChunk(sch)
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

	if !bytes.Equal(ch.Data(), u2.Chunk.Data()) {
		t.Fatalf("data mismatch %d %d", len(ch.Data()), u2.Chunk.Data())
	}

}
