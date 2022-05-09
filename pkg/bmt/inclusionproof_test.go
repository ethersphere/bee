// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package bmt_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/bmt/reference"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestInclusionProofCorrectness(t *testing.T) {
	testData := []byte("hello world")
	testData = append(testData, make([]byte, 4096-len(testData))...)

	verifySegments := func(t *testing.T, exp []string, found [][]byte) {
		var expSegments [][]byte
		for _, v := range exp {
			decoded, err := hex.DecodeString(v)
			if err != nil {
				t.Fatal(err)
			}
			expSegments = append(expSegments, decoded)
		}

		if len(expSegments) != len(found) {
			t.Fatal("incorrect no of proof segments")
		}

		for idx, v := range expSegments {
			if !bytes.Equal(v, found[idx]) {
				t.Fatal("incorrect segment in proof")
			}
		}

	}

	t.Run("proof for left most", func(t *testing.T) {
		segments, err := bmt.InclusionProofSegments(testData, 0, 4096)
		if err != nil {
			t.Fatal(err)
		}

		expSegmentStrings := []string{
			"0000000000000000000000000000000000000000000000000000000000000000",
			"ad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5",
			"b4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30",
			"21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85",
			"e58769b32a1beaf1ea27375a44095a0d1fb664ce2dd358e7fcbfb78c26a19344",
			"0eb01ebfc9ed27500cd4dfc979272d1f0913cc9f66540d7e8005811109e1cf2d",
			"887c22bd8750d34016ac3c66b5ff102dacdd73f6b014e710b51e8022af9a1968",
		}

		verifySegments(t, expSegmentStrings, segments)

	})

	t.Run("proof for right most", func(t *testing.T) {
		segments, err := bmt.InclusionProofSegments(testData, 127, 4096)
		if err != nil {
			t.Fatal(err)
		}

		expSegmentStrings := []string{
			"0000000000000000000000000000000000000000000000000000000000000000",
			"ad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5",
			"b4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30",
			"21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85",
			"e58769b32a1beaf1ea27375a44095a0d1fb664ce2dd358e7fcbfb78c26a19344",
			"0eb01ebfc9ed27500cd4dfc979272d1f0913cc9f66540d7e8005811109e1cf2d",
			"745bae095b6ff5416b4a351a167f731db6d6f5924f30cd88d48e74261795d27b",
		}

		verifySegments(t, expSegmentStrings, segments)

	})

	t.Run("proof for middle", func(t *testing.T) {
		segments, err := bmt.InclusionProofSegments(testData, 64, 4096)
		if err != nil {
			t.Fatal(err)
		}

		expSegmentStrings := []string{
			"0000000000000000000000000000000000000000000000000000000000000000",
			"ad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5",
			"b4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30",
			"21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85",
			"e58769b32a1beaf1ea27375a44095a0d1fb664ce2dd358e7fcbfb78c26a19344",
			"0eb01ebfc9ed27500cd4dfc979272d1f0913cc9f66540d7e8005811109e1cf2d",
			"745bae095b6ff5416b4a351a167f731db6d6f5924f30cd88d48e74261795d27b",
		}

		verifySegments(t, expSegmentStrings, segments)

	})

	t.Run("root hash calculation", func(t *testing.T) {
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

		segment := testData[64*bmt.SegmentSize : 65*bmt.SegmentSize]

		rootHash, err := bmt.RootHashFromInclusionProof(segments, segment, 64)
		if err != nil {
			t.Fatal(err)
		}

		expRootHash, err := hex.DecodeString("16043601ca4b598c08344a13d40019dfe94a109e9e38a2995352d6e3a5ebd7b5")
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(rootHash, expRootHash) {
			t.Fatal("incorrect root hash obtained")
		}

	})
}

func TestDifferentSegmentLengths(t *testing.T) {

	testDatas := []struct {
		name string
		data []byte
	}{
		{
			name: "zero data",
			data: make([]byte, 4096),
		},
		{
			name: "random data",
			data: randomBytes(t, time.Now().UnixNano()),
		},
	}

	for _, tc := range testDatas {
		t.Run(tc.name, func(t *testing.T) {
			testData := tc.data

			for _, count := range testSegmentCounts {
				t.Run(fmt.Sprintf("segments_%v", count), func(t *testing.T) {
					rbmt := reference.NewRefHasher(swarm.NewHasher(), count)

					// min size is 2 segments
					c := 2
					for ; c < count; c *= 2 {
					}

					maxDataLen := c * bmt.SegmentSize

					testIterData := testData[:maxDataLen]

					refNoMetaHash, err := rbmt.Hash(testIterData)
					if err != nil {
						t.Fatal(err)
					}

					// Get 5 random segments to prove. If we have less the 5 segments, verify
					// all of them
					segments := func() []int {
						if count < 5 {
							var all []int
							for i := 0; i < count; i++ {
								all = append(all, i)
							}
							return all
						}

						var randSegs []int
						for i := 0; i < 5; i++ {
							val := rand.Intn(count)
							randSegs = append(randSegs, val)
						}
						return randSegs
					}()

					for _, segmentIdx := range segments {
						t.Run(fmt.Sprintf("inclusion proofs are valid index %d", segmentIdx), func(t *testing.T) {
							proofSegments, err := bmt.InclusionProofSegments(testIterData, segmentIdx, maxDataLen)
							if err != nil {
								t.Fatal(err)
							}

							expSegments := int(math.Ceil(math.Log2(float64(count))))
							if expSegments == 0 {
								// if count is 1, we still have 2 segments so we need only 1
								expSegments = 1
							}

							if len(proofSegments) != expSegments {
								t.Fatalf("incorrect proof segments expected %d found %d", expSegments, len(proofSegments))
							}

							off := segmentIdx * bmt.SegmentSize
							rootHash, err := bmt.RootHashFromInclusionProof(proofSegments, testIterData[off:off+bmt.SegmentSize], segmentIdx)
							if err != nil {
								t.Fatal(err)
							}

							if !bytes.Equal(rootHash, refNoMetaHash) {
								t.Fatal("incorrect root hash from proof")
							}
						})
					}
				})
			}
		})
	}

}
