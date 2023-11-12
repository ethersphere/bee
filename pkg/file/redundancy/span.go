// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redundancy

import (
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
func DecodeSpan(span []byte) (int, []byte) {
	if !IsParityEncoded(span) {
		return 0, span
	}
	pByte := span[swarm.SpanSize-1]
	return int(pByte & ((1 << 7) - 1)), append(span[:swarm.SpanSize-1], 0)
}

// IsParityEncoded checks whether the parity is encoded in the span
// it assumes span is LittleEndian
func IsParityEncoded(span []byte) bool {
	return span[swarm.SpanSize-1] > 128
}
