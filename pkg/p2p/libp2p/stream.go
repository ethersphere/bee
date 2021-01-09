// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/libp2p/go-libp2p-core/network"
)

var _ p2p.Stream = (*stream)(nil)

type stream struct {
	network.Stream
	headers map[string][]byte
}

func NewStream(s network.Stream) p2p.Stream {
	return &stream{Stream: s}
}

func newStream(s network.Stream) *stream {
	return &stream{Stream: s}
}
func (s *stream) Headers() p2p.Headers {
	return s.headers
}

func (s *stream) FullClose() error {
	return s.Close() // this needs to change. since FullClose no longer exists
}
