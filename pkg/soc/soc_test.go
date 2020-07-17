// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package swarm contains most basic and general Swarm concepts.
package soc_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/swarm"
	bmtlegacy "github.com/ethersphere/bmt/legacy"
)

var (
	bmtPool = bmtlegacy.NewTreePool(swarm.NewHasher, swarm.Branches, bmtlegacy.PoolSize)
)


func newTestChunk(payload []byte) (swarm.Chunk, error) {
	span := int64(len(payload))
	bmtHasher := bmtlegacy.New(bmtPool)
	err := bmtHasher.SetSpan(span)
	if err != nil {
		return nil, err
	}
	_, err = bmtHasher.Write(payload)
	if err != nil {
		return nil, err
	}
	bmtSum := bmtHasher.Sum(nil)
	address := swarm.NewAddress(bmtSum)

	// write payload with span and data bytes
	spanBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(spanBytes, uint64(span))
	payloadWithSpan := append(spanBytes, payload...)

	return swarm.NewChunk(address, payloadWithSpan), nil
}

func newTestSocChunk(id soc.Id, ch swarm.Chunk, signer crypto.Signer) (swarm.Chunk, error) {
	s := soc.NewSoc(id, ch)
	err := s.AddSigner(signer)
	if err != nil {
		return nil, err
	}

	return s.ToChunk()
}

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
	ch, err := newTestChunk(payload)
	if err != nil {
		t.Fatal(err)
	}
	sch, err := newTestSocChunk(id, ch, signer)
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

	h := swarm.NewHasher()
	h.Write(id)
	h.Write(ch.Address().Bytes())
	toSignBytes := h.Sum(nil)

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
	ch, err := newTestChunk(payload)
	if err != nil {
		t.Fatal(err)
	}
	sch, err := newTestSocChunk(id, ch, signer)
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
}
