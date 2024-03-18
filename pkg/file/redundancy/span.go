// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redundancy

import (
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// EncodeLevel encodes used redundancy level for uploading into span keeping the real byte count for the chunk.
// assumes span is LittleEndian
func EncodeLevel(span []byte, level Level) {
	// set parity in the most signifact byte
	span[swarm.SpanSize-1] = uint8(level) | 1<<7 // p + 128
}

// DecodeSpan decodes the used redundancy level from span keeping the real byte count for the chunk.
// assumes span is LittleEndian
func DecodeSpan(span []byte) (Level, []byte) {
	spanCopy := make([]byte, swarm.SpanSize)
	copy(spanCopy, span)
	if !IsLevelEncoded(spanCopy) {
		return 0, spanCopy
	}
	pByte := spanCopy[swarm.SpanSize-1]
	return Level(pByte & ((1 << 7) - 1)), append(spanCopy[:swarm.SpanSize-1], 0)
}

// IsLevelEncoded checks whether the redundancy level is encoded in the span
// assumes span is LittleEndian
func IsLevelEncoded(span []byte) bool {
	return span[swarm.SpanSize-1] > 128
}
