package bmt

import "fmt"

const (
	DefaultMaxPayload = 4096
	SegmentSize       = 32
	SegmentPairSize   = 2 * SegmentSize
)

func constructBMT(payload []byte) ([][]byte, error) {
	if len(payload) > DefaultMaxPayload {
		return nil, fmt.Errorf("invalid payload length %d", len(payload))
	}

	// segments contains the data segments at each level of the BMT as we build it up
	// if we have less the 4096 bytes, we will pad the payload with zero bytes
	segments := append(payload, make([]byte, DefaultMaxPayload-len(payload))...)
	// levels will contain each level of the BMT as a byte array concatenated with all
	// the segments of that level up to the root hash of the BMT
	var levels [][]byte
	for len(segments) != SegmentSize {
		levels = append(levels, segments)
		// next will contain the segments obtained by hashing the contents of the
		// previous level. This will be the next level for the calculation
		next := make([]byte, len(segments)/2)

		for offset := 0; offset < len(segments); offset += SegmentPairSize {
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

func InclusionProofSegments(payload []byte, segmentIdx int) ([][]byte, error) {
	if segmentIdx*SegmentSize >= len(payload) {
		return nil, fmt.Errorf("invalid segment index %d in payload of length %d", segmentIdx, len(payload))
	}

	allLevels, err := constructBMT(payload)
	if err != nil {
		return nil, err
	}

	var sisterSegments [][]byte

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

func HashFromInclusionProof(segments [][]byte, segment []byte, segmentIdx int) ([]byte, error) {
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
