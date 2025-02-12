// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives_test

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/postage"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storageincentives"
	"github.com/ethersphere/bee/v2/pkg/storageincentives/redistribution"
	storer "github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	"github.com/google/go-cmp/cmp"
)

// Test asserts valid case for MakeInclusionProofs.
func TestMakeInclusionProofs_FLAKY(t *testing.T) {
	t.Parallel()

	anchor := testutil.RandBytes(t, 1)
	sample := storer.RandSample(t, anchor)

	_, err := storageincentives.MakeInclusionProofs(sample.Items, anchor, anchor)
	if err != nil {
		t.Fatal(err)
	}
}

//go:embed testdata/inclusion-proofs.json
var testData []byte

// Test asserts that MakeInclusionProofs will generate the same
// output for given sample.
func TestMakeInclusionProofsRegression(t *testing.T) {
	t.Parallel()

	const sampleSize = 16

	keyRaw := `00000000000000000000000000000000`
	privKey, err := crypto.DecodeSecp256k1PrivateKey([]byte(keyRaw))
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)

	stampID, _ := crypto.LegacyKeccak256([]byte("The Inverted Jenny"))
	index := []byte{0, 0, 0, 0, 0, 8, 3, 3}
	timestamp := []byte{0, 0, 0, 0, 0, 3, 3, 8}
	stamper := func(addr swarm.Address) *postage.Stamp {
		sig := postagetesting.MustNewValidSignature(signer, addr, stampID, index, timestamp)
		return postage.NewStamp(stampID, index, timestamp, sig)
	}

	anchor1 := big.NewInt(100).Bytes()
	anchor2 := big.NewInt(30).Bytes() // this anchor will pick chunks 3, 6, 15

	// generate chunks that will be used as sample
	sampleChunks := make([]swarm.Chunk, 0, sampleSize)
	for i := 0; i < sampleSize; i++ {
		ch, err := cac.New([]byte(fmt.Sprintf("Unstoppable data! Chunk #%d", i+1)))
		if err != nil {
			t.Fatal(err)
		}

		if i%2 == 0 {
			id, err := crypto.LegacyKeccak256([]byte(fmt.Sprintf("ID #%d", i+1)))
			if err != nil {
				t.Fatal(err)
			}

			socCh, err := soc.New(id, ch).Sign(signer)
			if err != nil {
				t.Fatal(err)
			}

			ch = socCh
		}

		ch = ch.WithStamp(stamper(ch.Address()))

		sampleChunks = append(sampleChunks, ch)
	}

	// make sample from chunks
	sample, err := storer.MakeSampleUsingChunks(sampleChunks, anchor1)
	if err != nil {
		t.Fatal(err)
	}

	// assert that sample chunk hash/address does not change
	sch, err := storageincentives.SampleChunk(sample.Items)
	if err != nil {
		t.Fatal(err)
	}
	if want := swarm.MustParseHexAddress("b012904b0c3e6462158b4416556caa888031a79bad46d2ffa7012408c9c38aa8"); !sch.Address().Equal(want) {
		t.Fatalf("expecting sample chunk address %v, got %v", want, sch.Address())
	}

	// assert that inclusion proofs values does not change
	proofs, err := storageincentives.MakeInclusionProofs(sample.Items, anchor1, anchor2)
	if err != nil {
		t.Fatal(err)
	}

	var expectedProofs redistribution.ChunkInclusionProofs

	err = json.Unmarshal(testData, &expectedProofs)
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(expectedProofs, proofs); diff != "" {
		t.Fatalf("unexpected inclusion proofs (-want +have):\n%s", diff)
	}
}

// Test asserts cases when MakeInclusionProofs should return error.
func TestMakeInclusionProofsExpectedError(t *testing.T) {
	t.Parallel()

	t.Run("invalid sample length", func(t *testing.T) {
		anchor := testutil.RandBytes(t, 8)
		sample := storer.RandSample(t, anchor)

		_, err := storageincentives.MakeInclusionProofs(sample.Items[:1], anchor, anchor)
		if err == nil {
			t.Fatal("expecting error")
		}
	})

	t.Run("empty anchor", func(t *testing.T) {
		sample := storer.RandSample(t, []byte{})

		_, err := storageincentives.MakeInclusionProofs(sample.Items[:1], []byte{}, []byte{})
		if err == nil {
			t.Fatal("expecting error")
		}
	})
}

// Tests asserts that creating sample chunk is valid for all lengths [1-MaxSampleSize]
func TestSampleChunk(t *testing.T) {
	t.Parallel()

	sample := storer.RandSample(t, nil)

	for i := 0; i < len(sample.Items); i++ {
		items := sample.Items[:i]

		chunk, err := storageincentives.SampleChunk(items)
		if err != nil {
			t.Fatal(err)
		}

		data := chunk.Data()[swarm.SpanSize:]
		pos := 0
		for _, item := range items {
			if !bytes.Equal(data[pos:pos+swarm.HashSize], item.ChunkAddress.Bytes()) {
				t.Error("expected chunk address")
			}
			pos += swarm.HashSize

			if !bytes.Equal(data[pos:pos+swarm.HashSize], item.TransformedAddress.Bytes()) {
				t.Error("expected transformed address")
			}
			pos += swarm.HashSize
		}

		if !chunk.Address().IsValidNonEmpty() {
			t.Error("address shouldn't be empty")
		}
	}
}

// Tests asserts that creating sample chunk should fail because it will exceed
// capacity of chunk data.
func TestSampleChunkExpectedError(t *testing.T) {
	t.Parallel()

	sampleItem := storer.RandSample(t, nil).Items[0]

	items := make([]storer.SampleItem, 65)
	for i := range items {
		items[i] = sampleItem
	}

	_, err := storageincentives.SampleChunk(items)
	if err == nil {
		t.Fatal("expecting error")
	}
}
