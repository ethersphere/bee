// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package soc_test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestNewSoc(t *testing.T) {
	payload := []byte("foo")
	ch, err := cac.New(payload)
	if err != nil {
		t.Fatal(err)
	}

	id := make([]byte, 32)
	s := soc.New(id, ch)

	// check soc fields
	if !bytes.Equal(s.ID(), id) {
		t.Fatalf("id mismatch. got %x want %x", s.ID(), id)
	}

	chunkData := s.WrappedChunk().Data()
	spanBytes := make([]byte, swarm.SpanSize)
	binary.LittleEndian.PutUint64(spanBytes, uint64(len(payload)))
	if !bytes.Equal(chunkData[:swarm.SpanSize], spanBytes) {
		t.Fatalf("span mismatch. got %x want %x", chunkData[:swarm.SpanSize], spanBytes)
	}

	if !bytes.Equal(chunkData[swarm.SpanSize:], payload) {
		t.Fatalf("payload mismatch. got %x want %x", chunkData[swarm.SpanSize:], payload)
	}
}

func TestNewSignedSoc(t *testing.T) {
	owner := common.HexToAddress("8d3766440f0d7b949a5e32995d09619a7f86e632")
	sig, err := hex.DecodeString("5acd384febc133b7b245e5ddc62d82d2cded9182d2716126cd8844509af65a053deb418208027f548e3e88343af6f84a8772fb3cebc0a1833a0ea7ec0c1348311b")
	if err != nil {
		t.Fatal(err)
	}

	payload := []byte("foo")
	ch, err := cac.New(payload)
	if err != nil {
		t.Fatal(err)
	}

	id := make([]byte, 32)
	s, err := soc.NewSigned(id, ch, owner.Bytes(), sig)
	if err != nil {
		t.Fatal(err)
	}

	// check signed soc fields
	if !bytes.Equal(s.ID(), id) {
		t.Fatalf("id mismatch. got %x want %x", s.ID(), id)
	}

	if !bytes.Equal(s.OwnerAddress(), owner.Bytes()) {
		t.Fatalf("owner mismatch. got %x want %x", s.OwnerAddress(), owner.Bytes())
	}

	if !bytes.Equal(s.Signature(), sig) {
		t.Fatalf("signature mismatch. got %x want %x", s.Signature(), sig)
	}

	chunkData := s.WrappedChunk().Data()
	spanBytes := make([]byte, swarm.SpanSize)
	binary.LittleEndian.PutUint64(spanBytes, uint64(len(payload)))
	if !bytes.Equal(chunkData[:swarm.SpanSize], spanBytes) {
		t.Fatalf("span mismatch. got %x want %x", chunkData[:swarm.SpanSize], spanBytes)
	}

	if !bytes.Equal(chunkData[swarm.SpanSize:], payload) {
		t.Fatalf("payload mismatch. got %x want %x", chunkData[swarm.SpanSize:], payload)
	}
}

// TestChunk verifies that the chunk create from the Soc object
// corresponds to the soc spec.
func TestChunk(t *testing.T) {
	owner := common.HexToAddress("8d3766440f0d7b949a5e32995d09619a7f86e632")
	sig, err := hex.DecodeString("5acd384febc133b7b245e5ddc62d82d2cded9182d2716126cd8844509af65a053deb418208027f548e3e88343af6f84a8772fb3cebc0a1833a0ea7ec0c1348311b")
	if err != nil {
		t.Fatal(err)
	}

	payload := []byte("foo")
	ch, err := cac.New(payload)
	if err != nil {
		t.Fatal(err)
	}

	id := make([]byte, 32)
	// creates a new soc
	s, err := soc.NewSigned(id, ch, owner.Bytes(), sig)
	if err != nil {
		t.Fatal(err)
	}

	sum, err := soc.Hash(id, owner.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	expectedSocAddress := swarm.NewAddress(sum)

	// creates soc chunk
	sch, err := s.Chunk()
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(sch.Address().Bytes(), expectedSocAddress.Bytes()) {
		t.Fatalf("soc address mismatch. got %x want %x", sch.Address().Bytes(), expectedSocAddress.Bytes())
	}

	chunkData := sch.Data()
	// verifies that id, signature, payload is in place in the soc chunk
	cursor := 0
	if !bytes.Equal(chunkData[cursor:soc.IdSize], id) {
		t.Fatalf("id mismatch. got %x want %x", chunkData[cursor:soc.IdSize], id)
	}
	cursor += soc.IdSize

	signature := chunkData[cursor : cursor+soc.SignatureSize]
	if !bytes.Equal(signature, sig) {
		t.Fatalf("signature mismatch. got %x want %x", signature, sig)
	}
	cursor += soc.SignatureSize

	spanBytes := make([]byte, swarm.SpanSize)
	binary.LittleEndian.PutUint64(spanBytes, uint64(len(payload)))
	if !bytes.Equal(chunkData[cursor:cursor+swarm.SpanSize], spanBytes) {
		t.Fatalf("span mismatch. got %x want %x", chunkData[cursor:cursor+swarm.SpanSize], spanBytes)
	}
	cursor += swarm.SpanSize

	if !bytes.Equal(chunkData[cursor:], payload) {
		t.Fatalf("payload mismatch. got %x want %x", chunkData[cursor:], payload)
	}
}

// TestSign tests whether a soc is correctly signed.
func TestSign(t *testing.T) {
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)

	payload := []byte("foo")
	ch, err := cac.New(payload)
	if err != nil {
		t.Fatal(err)
	}

	id := make([]byte, 32)
	// creates the soc
	s := soc.New(id, ch)

	// signs the chunk
	sch, err := s.Sign(signer)
	if err != nil {
		t.Fatal(err)
	}

	chunkData := sch.Data()
	// get signature in the chunk
	cursor := soc.IdSize
	signature := chunkData[cursor : cursor+soc.SignatureSize]

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

	// verifies if the owner matches
	recoveredEthereumAddress, err := soc.RecoverAddress(signature, toSignBytes)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(recoveredEthereumAddress, ethereumAddress) {
		t.Fatalf("signer address mismatch. got %x want %x", recoveredEthereumAddress, ethereumAddress)
	}
}

// TestFromChunk verifies that valid chunk data deserializes to
// a fully populated soc object.
func TestFromChunk(t *testing.T) {
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)

	payload := []byte("foo")
	ch, err := cac.New(payload)
	if err != nil {
		t.Fatal(err)
	}

	id := make([]byte, 32)
	s := soc.New(id, ch)

	sch, err := s.Sign(signer)
	if err != nil {
		t.Fatal(err)
	}

	s2, err := soc.FromChunk(sch)
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
	if !bytes.Equal(ownerEthereumAddress, s2.OwnerAddress()) {
		t.Fatalf("owner address mismatch. got %x want %x", ownerEthereumAddress, s2.OwnerAddress())
	}
}
