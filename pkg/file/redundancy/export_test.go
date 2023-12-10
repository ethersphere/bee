// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redundancy

// SetErasureEncoder changes erasureEncoderFunc to a new erasureEncoder facade
func SetErasureEncoder(f func(shards, parities int) (ErasureEncoder, error)) {
	erasureEncoderFunc = f
}

// GetErasureEncoder returns erasureEncoderFunc
func GetErasureEncoder() func(shards, parities int) (ErasureEncoder, error) {
	return erasureEncoderFunc
}
