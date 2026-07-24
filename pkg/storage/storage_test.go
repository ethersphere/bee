// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package storage_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/postage"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storage"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestIdentityAddress(t *testing.T) {
	t.Run("single owner chunk", func(t *testing.T) {
		t.Parallel()
		// Create a single owner chunk (SOC)
		owner := common.HexToAddress("8d3766440f0d7b949a5e32995d09619a7f86e632")
		// signature of hash(id + chunk address of foo)
		sig, err := hex.DecodeString("5acd384febc133b7b245e5ddc62d82d2cded9182d2716126cd8844509af65a053deb418208027f548e3e88343af6f84a8772fb3cebc0a1833a0ea7ec0c1348311b")
		if err != nil {
			t.Fatal(err)
		}
		id := make([]byte, swarm.HashSize)
		copy(id, []byte("id"))
		payload := []byte("foo")
		ch, err := cac.New(payload)
		if err != nil {
			t.Fatal(err)
		}
		sch, err := soc.NewSigned(id, ch, owner.Bytes(), sig)
		if err != nil {
			t.Fatal(err)
		}
		schChunk, err := sch.Chunk()
		if err != nil {
			t.Fatal(err)
		}
		schAddress, err := sch.Address()
		if err != nil {
			t.Fatal(err)
		}

		idAddr, err := storage.IdentityAddress(schChunk)
		if err != nil {
			t.Fatalf("IdentityAddress returned error: %v", err)
		}

		if idAddr.IsZero() {
			t.Fatalf("expected non-zero address, got zero address")
		}

		if idAddr.Equal(schAddress) {
			t.Fatalf("expected identity address to be different from SOC address")
		}
	})

	t.Run("content addressed chunk", func(t *testing.T) {
		t.Parallel()
		// Create a content addressed chunk (CAC)
		data := []byte("data")
		cacChunk, err := cac.New(data)
		if err != nil {
			t.Fatalf("failed to create content addressed chunk: %v", err)
		}

		// Call IdentityAddress with the CAC
		addr, err := storage.IdentityAddress(cacChunk)
		if err != nil {
			t.Fatalf("IdentityAddress returned error: %v", err)
		}

		// Verify the address matches the CAC address
		if !addr.Equal(cacChunk.Address()) {
			t.Fatalf("expected address %s, got %s", cacChunk.Address(), addr)
		}
	})
}

func TestChunkSum(t *testing.T) {
	t.Parallel()

	t.Run("cac is deterministic and 16 bytes", func(t *testing.T) {
		t.Parallel()
		ch := testingc.GenerateTestRandomChunk()

		sum, err := storage.ChunkSum(ch)
		if err != nil {
			t.Fatal(err)
		}
		if len(sum) != storage.ChunkSumSize {
			t.Fatalf("expected sum length %d, got %d", storage.ChunkSumSize, len(sum))
		}

		again, err := storage.ChunkSum(ch)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(sum, again) {
			t.Fatal("expected deterministic sum")
		}
	})

	t.Run("divergent socs share address but produce different sums", func(t *testing.T) {
		t.Parallel()

		privKey, err := crypto.GenerateSecp256k1Key()
		if err != nil {
			t.Fatal(err)
		}
		signer := crypto.NewDefaultSigner(privKey)
		id := make([]byte, swarm.HashSize)

		inner1, err := cac.New([]byte("content-one"))
		if err != nil {
			t.Fatal(err)
		}
		inner2, err := cac.New([]byte("content-two"))
		if err != nil {
			t.Fatal(err)
		}

		soc1, err := soc.New(id, inner1).Sign(signer)
		if err != nil {
			t.Fatal(err)
		}
		soc2, err := soc.New(id, inner2).Sign(signer)
		if err != nil {
			t.Fatal(err)
		}

		// same owner and id yield the same SOC address despite different content
		if !soc1.Address().Equal(soc2.Address()) {
			t.Fatalf("expected equal soc addresses, got %s and %s", soc1.Address(), soc2.Address())
		}

		// stamp both with the same stamp so batchID and stampHash are identical;
		// only the folded wrapped-CAC address should differ the sums.
		stamp := postagetesting.MustNewStamp()

		sum1, err := storage.ChunkSum(soc1.WithStamp(stamp))
		if err != nil {
			t.Fatal(err)
		}
		sum2, err := storage.ChunkSum(soc2.WithStamp(stamp))
		if err != nil {
			t.Fatal(err)
		}

		if bytes.Equal(sum1, sum2) {
			t.Fatal("divergent socs must produce different sums")
		}
	})

	t.Run("cac sum survives stamp marshal round-trip", func(t *testing.T) {
		t.Parallel()
		ch := testingc.GenerateTestRandomChunk()

		s1, err := storage.ChunkSum(ch)
		if err != nil {
			t.Fatal(err)
		}

		raw, err := ch.Stamp().MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		st := new(postage.Stamp)
		if err := st.UnmarshalBinary(raw); err != nil {
			t.Fatal(err)
		}

		s2, err := storage.ChunkSum(swarm.NewChunk(ch.Address(), ch.Data()).WithStamp(st))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(s1, s2) {
			t.Fatal("sum must be stable across stamp marshal round-trip")
		}
	})
}

// FuzzChunkSum exercises the checksum computation with fully arbitrary chunk
// data and stamp fields. Pullsync recomputes the sum on delivered chunks
// before their validity is checked, so this path processes attacker-controlled
// bytes and must never panic. It also pins the equivalence of ChunkSum and
// ChunkSumFromParts whenever a sum is computable at all.
func FuzzChunkSum(f *testing.F) {
	inner, err := cac.New([]byte("seed payload"))
	if err != nil {
		f.Fatal(err)
	}
	validSoc := testingc.GenerateTestRandomSoChunk(f, inner)
	f.Add(validSoc.Data(), []byte("batch"), []byte("index"), []byte("ts"), []byte("sig"))
	f.Add([]byte{}, []byte{}, []byte{}, []byte{}, []byte{})
	f.Add(make([]byte, swarm.ChunkWithSpanSize), make([]byte, 32), make([]byte, 8), make([]byte, 8), make([]byte, 65))

	f.Fuzz(func(t *testing.T, data, batchID, index, ts, sig []byte) {
		if len(data) > swarm.SocMaxChunkSize {
			t.Skip()
		}

		hasher := swarm.NewHasher()
		_, _ = hasher.Write(data)
		ch := swarm.NewChunk(swarm.NewAddress(hasher.Sum(nil)), data).
			WithStamp(postage.NewStamp(batchID, index, ts, sig))

		sum, err := storage.ChunkSum(ch) // must not panic on any input
		if err != nil {
			return
		}
		if len(sum) != storage.ChunkSumSize {
			t.Fatalf("sum length %d, want %d", len(sum), storage.ChunkSumSize)
		}

		stampHash, err := ch.Stamp().Hash()
		if err != nil {
			t.Fatalf("sum computed but stamp hash failed: %v", err)
		}
		fromParts, err := storage.ChunkSumFromParts(ch.Stamp().BatchID(), stampHash, ch)
		if err != nil {
			t.Fatalf("sum computed but parts variant failed: %v", err)
		}
		if !bytes.Equal(sum, fromParts) {
			t.Fatal("ChunkSum and ChunkSumFromParts disagree")
		}
	})
}
