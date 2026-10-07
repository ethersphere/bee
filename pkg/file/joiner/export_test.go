// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package joiner

// SubtrieSection exposes subtrieSection for tests.
func SubtrieSection(maxBranching, refLength, startIdx, payloadSize, parities int, subtrieSize int64) int64 {
	j := &joiner{maxBranching: maxBranching, refLength: refLength}
	return j.subtrieSection(startIdx, payloadSize, parities, subtrieSize)
}
