// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package content

import (
	"github.com/ethersphere/bee/pkg/swarm"
)

// Valid checks the validity.
func Valid(ch swarm.Chunk) (valid bool) {
	rch, err := contentChunkFromBytes(ch.Data())
	if err != nil {
		return false
	}
	return ch.Address().Equal(rch.Address())
}
