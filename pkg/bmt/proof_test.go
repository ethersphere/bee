// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt_test

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bmt"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestProofCorrectness(t *testing.T) {
	t.Parallel()

	testData := []byte("hello world")
	testDataPadded := make([]byte, swarm.ChunkSize)
	copy(testDataPadded, testData)

	verifySegments := func(t *testing.T, exp []string, found [][]byte) {
		t.Helper()

		var expSegments [][]byte
		for _, v := range exp {
			decoded, err := hex.DecodeString(v)
			if err != nil {
				t.Fatal(err)
			}
			expSegments = append(expSegments, decoded)
		}

		if len(expSegments) != len(found) {
			t.Fatal("incorrect no of proof segments", len(found))
		}

		for idx, v := range expSegments {
			if !bytes.Equal(v, found[idx]) {
				t.Fatal("incorrect segment in proof")
			}
		}
	}

	pool := bmt.NewPool(bmt.NewConf(swarm.NewHasher, 128, 128))
	hh := pool.Get()
	t.Cleanup(func() {
		pool.Put(hh)
	})
	hh.SetHeaderInt64(4096)

	_, err := hh.Write(testData)
	if err != nil {
		t.Fatal(err)
	}
	pr := bmt.Prover{hh}
	rh, err := pr.Hash(nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("proof for left most", func(t *testing.T) {
		t.Parallel()

		proof := pr.Proof(0)

		expSegmentStrings := []string{
			"0000000000000000000000000000000000000000000000000000000000000000",
			"ad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5",
			"b4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30",
			"21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85",
			"e58769b32a1beaf1ea27375a44095a0d1fb664ce2dd358e7fcbfb78c26a19344",
			"0eb01ebfc9ed27500cd4dfc979272d1f0913cc9f66540d7e8005811109e1cf2d",
			"887c22bd8750d34016ac3c66b5ff102dacdd73f6b014e710b51e8022af9a1968",
		}

		verifySegments(t, expSegmentStrings, proof.ProofSegments)

		if !bytes.Equal(proof.ProveSegment, testDataPadded[:hh.Size()]) {
			t.Fatal("section incorrect")
		}

		if !bytes.Equal(proof.Span, bmt.LengthToSpan(4096)) {
			t.Fatal("incorrect span")
		}
	})

	t.Run("proof for right most", func(t *testing.T) {
		t.Parallel()

		proof := pr.Proof(127)

		expSegmentStrings := []string{
			"0000000000000000000000000000000000000000000000000000000000000000",
			"ad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5",
			"b4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30",
			"21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85",
			"e58769b32a1beaf1ea27375a44095a0d1fb664ce2dd358e7fcbfb78c26a19344",
			"0eb01ebfc9ed27500cd4dfc979272d1f0913cc9f66540d7e8005811109e1cf2d",
			"745bae095b6ff5416b4a351a167f731db6d6f5924f30cd88d48e74261795d27b",
		}

		verifySegments(t, expSegmentStrings, proof.ProofSegments)

		if !bytes.Equal(proof.ProveSegment, testDataPadded[127*hh.Size():]) {
			t.Fatal("section incorrect")
		}

		if !bytes.Equal(proof.Span, bmt.LengthToSpan(4096)) {
			t.Fatal("incorrect span")
		}
	})

	t.Run("proof for middle", func(t *testing.T) {
		t.Parallel()

		proof := pr.Proof(64)

		expSegmentStrings := []string{
			"0000000000000000000000000000000000000000000000000000000000000000",
			"ad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5",
			"b4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30",
			"21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85",
			"e58769b32a1beaf1ea27375a44095a0d1fb664ce2dd358e7fcbfb78c26a19344",
			"0eb01ebfc9ed27500cd4dfc979272d1f0913cc9f66540d7e8005811109e1cf2d",
			"745bae095b6ff5416b4a351a167f731db6d6f5924f30cd88d48e74261795d27b",
		}

		verifySegments(t, expSegmentStrings, proof.ProofSegments)

		if !bytes.Equal(proof.ProveSegment, testDataPadded[64*hh.Size():65*hh.Size()]) {
			t.Fatal("section incorrect")
		}

		if !bytes.Equal(proof.Span, bmt.LengthToSpan(4096)) {
			t.Fatal("incorrect span")
		}
	})

	t.Run("root hash calculation", func(t *testing.T) {
		t.Parallel()

		segmentStrings := []string{
			"0000000000000000000000000000000000000000000000000000000000000000",
			"ad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5",
			"b4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30",
			"21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85",
			"e58769b32a1beaf1ea27375a44095a0d1fb664ce2dd358e7fcbfb78c26a19344",
			"0eb01ebfc9ed27500cd4dfc979272d1f0913cc9f66540d7e8005811109e1cf2d",
			"745bae095b6ff5416b4a351a167f731db6d6f5924f30cd88d48e74261795d27b",
		}

		var segments [][]byte
		for _, v := range segmentStrings {
			decoded, err := hex.DecodeString(v)
			if err != nil {
				t.Fatal(err)
			}
			segments = append(segments, decoded)
		}

		segment := testDataPadded[64*hh.Size() : 65*hh.Size()]

		rootHash, err := pr.Verify(64, bmt.Proof{
			ProveSegment:  segment,
			ProofSegments: segments,
			Span:          bmt.LengthToSpan(4096),
		})
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(rootHash, rh) {
			t.Fatal("incorrect root hash obtained")
		}
	})
}

func TestProof(t *testing.T) {
	t.Parallel()

	// BMT segment inclusion proofs
	// Usage
	buf := make([]byte, 4096)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		t.Fatal(err)
	}

	pool := bmt.NewPool(bmt.NewConf(swarm.NewHasher, 128, 128))
	hh := pool.Get()
	t.Cleanup(func() {
		pool.Put(hh)
	})
	hh.SetHeaderInt64(4096)

	_, err = hh.Write(buf)
	if err != nil {
		t.Fatal(err)
	}

	rh, err := hh.Hash(nil)
	pr := bmt.Prover{hh}
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 128; i++ {
		t.Run(fmt.Sprintf("segmentIndex %d", i), func(t *testing.T) {
			t.Parallel()

			proof := pr.Proof(i)

			h := pool.Get()
			defer pool.Put(h)

			root, err := bmt.Prover{h}.Verify(i, proof)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(rh, root) {
				t.Fatalf("incorrect hash. wanted %x, got %x.", rh, root)
			}
		})
	}
}
