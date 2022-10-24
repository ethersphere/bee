// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pushsync provides the pushsync protocol
// implementation.
package streamcache

import (
	"github.com/ethersphere/bee/pkg/p2p"
)

type CachedStream struct {
	p2p.Stream
}

func (s *CachedStream) Headers() p2p.Headers {
	return s.Stream.Headers()
}

func (s *CachedStream) ResponseHeaders() p2p.Headers {
	return s.Stream.ResponseHeaders()
}

// FullClose hijacks the call and does nothing
func (s *CachedStream) FullClose() error {
	return nil
}
