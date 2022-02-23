// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package soc_test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestNew(t *testing.T) {
	payload := []byte("foo")
	ch, err := cac.New(payload)
	if err != nil {
		t.Fatal(err)
	}

	id := make([]byte, swarm.HashSize)
	s := soc.New(id, ch)

	// check SOC fields
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

func TestNewSigned(t *testing.T) {
	owner := common.HexToAddress("8d3766440f0d7b949a5e32995d09619a7f86e632")
	// signature of hash(id + chunk address of foo)
	sig, err := hex.DecodeString("5acd384febc133b7b245e5ddc62d82d2cded9182d2716126cd8844509af65a053deb418208027f548e3e88343af6f84a8772fb3cebc0a1833a0ea7ec0c1348311b")
	if err != nil {
		t.Fatal(err)
	}

	payload := []byte("foo")
	ch, err := cac.New(payload)
	if err != nil {
		t.Fatal(err)
	}

	id := make([]byte, swarm.HashSize)
	s, err := soc.NewSigned(id, ch, owner.Bytes(), sig)
	if err != nil {
		t.Fatal(err)
	}

	// check signed SOC fields
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

// TestChunk verifies that the chunk created from the SOC object
// corresponds to the SOC spec.
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

	id := make([]byte, swarm.HashSize)
	// creates a new signed SOC
	s, err := soc.NewSigned(id, ch, owner.Bytes(), sig)
	if err != nil {
		t.Fatal(err)
	}

	sum, err := soc.Hash(id, owner.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	expectedSOCAddress := swarm.NewAddress(sum)

	// creates SOC chunk
	sch, err := s.Chunk()
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(sch.Address().Bytes(), expectedSOCAddress.Bytes()) {
		t.Fatalf("soc address mismatch. got %x want %x", sch.Address().Bytes(), expectedSOCAddress.Bytes())
	}

	chunkData := sch.Data()
	// verifies that id, signature, payload is in place in the SOC chunk
	cursor := 0
	if !bytes.Equal(chunkData[cursor:swarm.HashSize], id) {
		t.Fatalf("id mismatch. got %x want %x", chunkData[cursor:swarm.HashSize], id)
	}
	cursor += swarm.HashSize

	signature := chunkData[cursor : cursor+swarm.SocSignatureSize]
	if !bytes.Equal(signature, sig) {
		t.Fatalf("signature mismatch. got %x want %x", signature, sig)
	}
	cursor += swarm.SocSignatureSize

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

func TestChunkErrorWithoutOwner(t *testing.T) {
	payload := []byte("foo")
	ch, err := cac.New(payload)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, swarm.HashSize)

	// creates a new soc
	s := soc.New(id, ch)

	_, err = s.Chunk()
	if !errors.Is(err, soc.ErrInvalidAddress) {
		t.Fatalf("expect error. got `%v` want `%v`", err, soc.ErrInvalidAddress)
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

	id := make([]byte, swarm.HashSize)
	// creates the soc
	s := soc.New(id, ch)

	// signs the chunk
	sch, err := s.Sign(signer)
	if err != nil {
		t.Fatal(err)
	}

	chunkData := sch.Data()
	// get signature in the chunk
	cursor := swarm.HashSize
	signature := chunkData[cursor : cursor+swarm.SocSignatureSize]

	// get the public key of the signer
	publicKey, err := signer.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	owner, err := crypto.NewEthereumAddress(*publicKey)
	if err != nil {
		t.Fatal(err)
	}

	toSignBytes, err := soc.Hash(id, ch.Address().Bytes())
	if err != nil {
		t.Fatal(err)
	}

	// verifies if the owner matches
	recoveredOwner, err := soc.RecoverAddress(signature, toSignBytes)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(recoveredOwner, owner) {
		t.Fatalf("owner address mismatch. got %x want %x", recoveredOwner, owner)
	}
}

// TestFromChunk verifies that valid chunk data deserializes to
// a fully populated soc object.
func TestFromChunk(t *testing.T) {
	socAddress := swarm.MustParseHexAddress("9d453ebb73b2fedaaf44ceddcf7a0aa37f3e3d6453fea5841c31f0ea6d61dc85")

	// signed soc chunk of:
	// id: 0
	// wrapped chunk of: `foo`
	// owner: 0x8d3766440f0d7b949a5e32995d09619a7f86e632
	sch := swarm.NewChunk(socAddress, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 90, 205, 56, 79, 235, 193, 51, 183, 178, 69, 229, 221, 198, 45, 130, 210, 205, 237, 145, 130, 210, 113, 97, 38, 205, 136, 68, 80, 154, 246, 90, 5, 61, 235, 65, 130, 8, 2, 127, 84, 142, 62, 136, 52, 58, 246, 248, 74, 135, 114, 251, 60, 235, 192, 161, 131, 58, 14, 167, 236, 12, 19, 72, 49, 27, 3, 0, 0, 0, 0, 0, 0, 0, 102, 111, 111})

	cursor := swarm.HashSize + swarm.SocSignatureSize
	data := sch.Data()
	id := data[:swarm.HashSize]
	sig := data[swarm.HashSize:cursor]
	chunkData := data[cursor:]

	chunkAddress := swarm.MustParseHexAddress("2387e8e7d8a48c2a9339c97c1dc3461a9a7aa07e994c5cb8b38fd7c1b3e6ea48")
	ch := swarm.NewChunk(chunkAddress, chunkData)

	signedDigest, err := soc.Hash(id, ch.Address().Bytes())
	if err != nil {
		t.Fatal(err)
	}

	ownerAddress, err := soc.RecoverAddress(sig, signedDigest)
	if err != nil {
		t.Fatal(err)
	}

	// attempt to recover soc from signed chunk
	recoveredSOC, err := soc.FromChunk(sch)
	if err != nil {
		t.Fatal(err)
	}

	// owner matching means the address was successfully recovered from
	// payload and signature
	if !bytes.Equal(recoveredSOC.OwnerAddress(), ownerAddress) {
		t.Fatalf("owner address mismatch. got %x want %x", recoveredSOC.OwnerAddress(), ownerAddress)
	}

	if !bytes.Equal(recoveredSOC.ID(), id) {
		t.Fatalf("id mismatch. got %x want %x", recoveredSOC.ID(), id)
	}

	if !bytes.Equal(recoveredSOC.Signature(), sig) {
		t.Fatalf("signature mismatch. got %x want %x", recoveredSOC.Signature(), sig)
	}

	if !ch.Equal(recoveredSOC.WrappedChunk()) {
		t.Fatalf("wrapped chunk mismatch. got %s want %s", recoveredSOC.WrappedChunk().Address(), ch.Address())
	}
}

func TestCreateAddress(t *testing.T) {
	id := make([]byte, swarm.HashSize)
	owner := common.HexToAddress("8d3766440f0d7b949a5e32995d09619a7f86e632")
	socAddress := swarm.MustParseHexAddress("9d453ebb73b2fedaaf44ceddcf7a0aa37f3e3d6453fea5841c31f0ea6d61dc85")

	addr, err := soc.CreateAddress(id, owner.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !addr.Equal(socAddress) {
		t.Fatalf("soc address mismatch. got %s want %s", addr, socAddress)
	}
}

func TestRecoverAddress(t *testing.T) {
	owner := common.HexToAddress("8d3766440f0d7b949a5e32995d09619a7f86e632")
	id := make([]byte, swarm.HashSize)
	chunkAddress := swarm.MustParseHexAddress("2387e8e7d8a48c2a9339c97c1dc3461a9a7aa07e994c5cb8b38fd7c1b3e6ea48")
	signedDigest, err := soc.Hash(id, chunkAddress.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	sig, err := hex.DecodeString("5acd384febc133b7b245e5ddc62d82d2cded9182d2716126cd8844509af65a053deb418208027f548e3e88343af6f84a8772fb3cebc0a1833a0ea7ec0c1348311b")
	if err != nil {
		t.Fatal(err)
	}

	// attempt to recover address from signature
	addr, err := soc.RecoverAddress(sig, signedDigest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(addr, owner.Bytes()) {
		t.Fatalf("owner address mismatch. got %x want %x", addr, owner.Bytes())
	}
}
