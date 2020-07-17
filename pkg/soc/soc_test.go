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

func newTestSocChunk(id soc.Id, payload []byte, signer crypto.Signer) (swarm.Chunk, error) {
	span := len(payload)

	bmtPool := bmtlegacy.NewTreePool(swarm.NewHasher, swarm.Branches, bmtlegacy.PoolSize)
	bmtHasher := bmtlegacy.New(bmtPool)

	err := bmtHasher.SetSpan(int64(span))
	if err != nil {
		return nil, err
	}
	_, err = bmtHasher.Write(payload)
	if err != nil {
		return nil, err
	}
	bmtSum := bmtHasher.Sum(nil)
	address := swarm.NewAddress(bmtSum)

	ch := swarm.NewChunk(address, payload)
	u := soc.NewSoc(id, ch)
	err = u.AddSigner(signer)
	if err != nil {
		return nil, err
	}
	return u.CreateChunk()
}

// TestCreateChunk verifies that the chunk create from the update object
// corresponds to the soc spec.
func TestCreateChunk(t *testing.T) {

	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)
	
	id := make([]byte, 32)

	payload := []byte("foo")
	ch, err := newTestSocChunk(id, payload, signer)
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
	cursor += soc.SignatureSize
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

	toSignBytes := append(id, payload...)

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

	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)

	id := make([]byte, 32)

	payload := []byte("foo")
	ch, err := newTestSocChunk(id, payload, signer)
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
