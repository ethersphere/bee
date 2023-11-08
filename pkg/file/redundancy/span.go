// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redundancy

import (
	"encoding/binary"

	"github.com/ethersphere/bee/pkg/swarm"
)

// EncodeParity encodes parities into span keeping the real byte count for the chunk.
// it assumes span is LittleEndian
func EncodeParity(span []byte, parities int) {
	// set parity in the most signifact byte
	span[swarm.SpanSize-1] = uint8(parities) | 1<<7 // p + 128
}

// DecodeSpan decodes parity from span keeping the real byte count for the chunk.
// it assumes span is LittleEndian
func DecodeSpan(span []byte) (int, uint64) {
	return int(span[swarm.SpanSize-1] & ((1 << 7) - 1)), binary.LittleEndian.Uint64(span[:swarm.SpanSize-1])
}
