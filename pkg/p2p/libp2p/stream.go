// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"github.com/libp2p/go-libp2p-core/helpers"
	"github.com/libp2p/go-libp2p-core/network"
)

type stream struct {
	network.Stream
}

func (s *stream) FullClose() error {
	return helpers.FullClose(s)
}

func newStream(s network.Stream) *stream {
	return &stream{Stream: s}
}
