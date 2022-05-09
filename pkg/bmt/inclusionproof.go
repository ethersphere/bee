package bmt

import (
	"fmt"
)

const (
	DefaultMaxPayload = 4096
	SegmentSize       = 32
	SegmentPairSize   = 2 * SegmentSize
)

// constructBMT will construct all the levels of the BMT for the chunk payload. The
// levels will be returned as an array of byte slices, with the segments/hashes at
// each level concatenated in one slice. Each index in the levels will contain 1 level
// of the tree
//                   Root hash                                      levels[top]
//                       .                                             .
//                       .                                             .
//                       .                                             .
//      H0123..........................  H(n-2, n-1, n, n+1)        levels[2]
//   H01     H23 .............................  H(n, n+1)           levels[1]
// s0  s1  s2  s3............................s(n)  s(n+1)           levels[0]
// Each segment and hash is the same 32bytes. This allows for random access in the
// byte slice for any segment/hash at any level
func constructBMT(payload []byte, maxPayloadLength int) ([][]byte, error) {
	if len(payload) > maxPayloadLength {
		return nil, fmt.Errorf("invalid payload length %d", len(payload))
	}

	// segments contains the data segments at each level of the BMT as we build it up
	// if we have less the 4096 bytes, we will pad the payload with zero bytes
	segments := append(payload, make([]byte, maxPayloadLength-len(payload))...)
	// levels will contain each level of the BMT as a byte array concatenated with all
	// the segments of that level up to the root hash of the BMT
	var levels [][]byte
	// Once we reach the root hash, we will have just 32bytes left
	for len(segments) != SegmentSize {
		levels = append(levels, segments)
		// next will contain the segments obtained by hashing the contents of the
		// previous level. This will be the next level for the calculation
		next := make([]byte, len(segments)/2)

		for offset := 0; offset < len(segments); offset += SegmentPairSize {
			// TODO: this can be done in parallel without any sync, possible performance enhancement
			hash, err := sha3hash(segments[offset : offset+SegmentPairSize])
			if err != nil {
				return nil, err
			}
			n := copy(next[offset/2:], hash)
			if n != SegmentSize {
				return nil, fmt.Errorf("invalid no of bytes copied after hash %d", n)
			}
		}

		segments = next
	}

	levels = append(levels, segments)
	return levels, nil
}

// InclusionProofSegments returns all the segments/hashes of the BMT required to prove
// a particular segment is part of the payload. The BMT is constructed by hashing the
// segments (32 bytes) in pairs and then hashing the resulting hashes together in pairs
// again till we reach the root hash. This way we don't need to know the entire chunk to prove
// a particular segment as long as we have all the sister hashes of the BMT at each level.
// Due to the nature of the BMT the index is related to how we go about selecting the
// segments/hashes from the BMT. In the byte representation of the index, each bit
// will correspond to location (left child or right). For generating the proof we need
// the other child. If the bit is 0 (index is even), it means the current index is the left child and
// we need the right child which is the next index in the byte slice of BMT at that level and vice versa.
// For a detailed pictorial description, please refer The Book of Swarm (page 40).
func InclusionProofSegments(payload []byte, segmentIdx int, maxPayloadLength int) ([][]byte, error) {
	if segmentIdx*SegmentSize >= len(payload) {
		return nil, fmt.Errorf("invalid segment index %d in payload of length %d", segmentIdx, len(payload))
	}

	allLevels, err := constructBMT(payload, maxPayloadLength)
	if err != nil {
		return nil, err
	}

	var sisterSegments [][]byte
	// the top level is the root hash which should not be included in the proof, so for
	// getting the proof segments the top is 1 level below the root hash
	top := len(allLevels) - 1
	for level := 0; level < top; level++ {
		sisterSegmentOffset := 0

		// if the segmentIdx on a particular level is even, we are basically at the
		// left child of the branch. For the proof we need the other segment which is
		// the next one
		if segmentIdx%2 == 0 {
			sisterSegmentOffset = (segmentIdx + 1) * SegmentSize
		} else {
			sisterSegmentOffset = (segmentIdx - 1) * SegmentSize
		}

		sisterSegments = append(
			sisterSegments,
			allLevels[level][sisterSegmentOffset:sisterSegmentOffset+SegmentSize],
		)

		segmentIdx >>= 1
	}

	return sisterSegments, nil
}

// RootHashFromInclusionProof uses the proof segments and calculated the root hash.
// As mentioned earlier, the index relates to the position of the segment in the BMT
// and is used to determine whether the particular segment needs to be merged from
// left or right.
func RootHashFromInclusionProof(segments [][]byte, segment []byte, segmentIdx int) ([]byte, error) {
	isRight := func(idx int) bool { return idx%2 == 0 }

	calculatedHash := make([]byte, SegmentSize)
	copy(calculatedHash, segment)

	for _, proofSegment := range segments {
		var err error

		if isRight(segmentIdx) {
			calculatedHash, err = sha3hash(calculatedHash, proofSegment)
		} else {
			calculatedHash, err = sha3hash(proofSegment, calculatedHash)
		}
		if err != nil {
			return nil, err
		}
		segmentIdx >>= 1
	}

	return calculatedHash, nil
}
