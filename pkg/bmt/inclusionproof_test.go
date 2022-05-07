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

	testData := randomBytes(t, time.Now().UnixNano())

	for _, count := range testSegmentCounts {
		t.Run(fmt.Sprintf("segments_%v", count), func(t *testing.T) {
			rbmt := reference.NewRefHasher(swarm.NewHasher(), count)

			c := 2
			for ; c < count; c *= 2 {
			}

			maxDataLen := c * bmt.SegmentSize

			testIterData := testData[:maxDataLen]

			refNoMetaHash, err := rbmt.Hash(testIterData)
			if err != nil {
				t.Fatal(err)
			}

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

					fmt.Println("No of segments", len(proofSegments), count)

					expSegments := int(math.Ceil(math.Log2(float64(count))))
					if expSegments == 0 {
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

}
