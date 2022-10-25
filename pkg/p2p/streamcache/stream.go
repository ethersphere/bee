// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pushsync provides the pushsync protocol
// implementation.
package streamcache

import (
	"sync"

	"github.com/ethersphere/bee/pkg/p2p"
)

type CachedStream struct {
	mu     sync.Mutex
	stream p2p.Stream
}

func (s *CachedStream) Headers() p2p.Headers {
	return s.stream.Headers()
}

func (s *CachedStream) ResponseHeaders() p2p.Headers {
	return s.stream.ResponseHeaders()
}

// FullClose hijacks the call and does nothing
func (s *CachedStream) FullClose() error {
	return nil
}

func (s *CachedStream) Read(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.stream.Read(p)
}

func (s *CachedStream) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.stream.Write(p)
}

func (s *CachedStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.stream.Close()
}

func (s *CachedStream) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.stream.Reset()
}
