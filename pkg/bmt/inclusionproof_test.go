package bmt_test

import (
	"bytes"
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/bmt/reference"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestInclusionProofs(t *testing.T) {

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
