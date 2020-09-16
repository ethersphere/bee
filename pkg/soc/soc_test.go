// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package swarm contains most basic and general Swarm concepts.
package soc_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
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

// TestSelfAddressedEnvelope check the creation of a self addressed soc envelope chunk
func TestSelfAddressedEnvelope(t *testing.T) {
	payload := make([]byte, swarm.ChunkSize)
	_, err := rand.Read(payload)
	if err != nil {
		t.Fatal(err)
	}
	c1, err := content.NewChunk(payload)
	if err != nil {
		t.Fatal(err)
	}

	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)
	publicKey, err := signer.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	overlayBytes, err := crypto.NewEthereumAddress(*publicKey)
	if err != nil {
		t.Fatal(err)
	}
	overlayAddres := swarm.NewAddress(overlayBytes)
	owner, err := soc.NewOwner(overlayBytes)
	if err != nil {
		t.Fatal(err)
	}
	// with target len 1
	ctx := context.Background()
	envelope1, err := soc.CreateSelfAddressedEnvelope(ctx, owner, overlayAddres, 1, c1.Address(), signer)
	if err != nil {
		t.Fatal(err)
	}
	if envelope1.Address().Bytes()[0] != overlayAddres.Bytes()[0] {
		t.Fatal(" invalid prefix mined")
	}

	// with target len 2
	envelope2, err := soc.CreateSelfAddressedEnvelope(ctx, owner, overlayAddres, 2, c1.Address(), signer)
	if err != nil {
		t.Fatal(err)
	}
	if envelope2.Address().Bytes()[0] != overlayAddres.Bytes()[0] {
		t.Fatal(" invalid prefix mined")
	}
}

// BenchmarkEnvelopeMiner benchmarks the mining of the ID to create a self addressed envelope
func BenchmarkEnvelopeMiner(b *testing.B) {
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		b.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)
	publicKey, err := signer.PublicKey()
	if err != nil {
		b.Fatal(err)
	}
	overlayBytes, err := crypto.NewEthereumAddress(*publicKey)
	if err != nil {
		b.Fatal(err)
	}
	overlay := swarm.NewAddress(overlayBytes)
	owner, err := soc.NewOwner(overlayBytes)
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()
	cases := []struct {
		payloadId string
		targetLen int
	}{
		{"b93d610abfbc7e77ce64ecb0aef3c4f40914894702a3903ef8b857b89e74748d", 1},
		{"8df825f58ede92e2ef6a1bbfb1ebeb6268653e748a87f4ed439216a83147457b", 1},
		{"7a5f7fff51debee4133a4876f4e1c8e9899d177f63df703ea334f1371c9e26bc", 1},
		{"b93d610abfbc7e77ce64ecb0aef3c4f40914894702a3903ef8b857b89e74748d", 2},
		{"8df825f58ede92e2ef6a1bbfb1ebeb6268653e748a87f4ed439216a83147457b", 2},
		{"7a5f7fff51debee4133a4876f4e1c8e9899d177f63df703ea334f1371c9e26bc", 2},
		{"b93d610abfbc7e77ce64ecb0aef3c4f40914894702a3903ef8b857b89e74748d", 3},
		{"8df825f58ede92e2ef6a1bbfb1ebeb6268653e748a87f4ed439216a83147457b", 3},
		{"7a5f7fff51debee4133a4876f4e1c8e9899d177f63df703ea334f1371c9e26bc", 3},
	}
	for _, c := range cases {
		address, err := swarm.ParseHexAddress(c.payloadId)
		if err != nil {
			b.Fatal(err)
		}
		name := fmt.Sprintf("payload Id:%s, targetLen:%d", c.payloadId, c.targetLen)
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := soc.CreateSelfAddressedEnvelope(ctx, owner, overlay, c.targetLen, address, signer); err != nil {
					b.Fatal(err)
				}
			}
		})
	}

}
