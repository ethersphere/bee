// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package content

import (
	"bytes"

	"github.com/ethersphere/bee/pkg/bmtpool"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Valid checks whether the given chunk is a valid content-addressed chunk.
func Valid(c swarm.Chunk) bool {
	data := c.Data()
	if len(data) < swarm.SpanSize {
		return false
	}

	span := data[:swarm.SpanSize]
	content := data[swarm.SpanSize:]

	hasher := bmtpool.Get()
	defer bmtpool.Put(hasher)

	// execute hash, compare and return result
	err := hasher.SetSpanBytes(span)
	if err != nil {
		return false
	}
	_, err = hasher.Write(content)
	if err != nil {
		return false
	}
	s := hasher.Sum(nil)

	return bytes.Equal(s, c.Address().Bytes())
}
