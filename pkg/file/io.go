// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package file

import (
	"io"
)

// simpleJoinerReadCloser wraps a byte slice in a io.ReadCloser implementation.
type simpleReadCloser struct {
	buffer []byte
	cursor int
}

func NewSimpleReadCloser(buffer []byte) io.ReadCloser {
	return &simpleReadCloser{
		buffer: buffer,
	}
}

// Read implements io.Reader.
func (s *simpleReadCloser) Read(b []byte) (int, error) {
	if s.cursor == len(s.buffer) {
		return 0, io.EOF
	}
	copy(b, s.buffer)
	s.cursor += len(s.buffer)
	return len(s.buffer), nil
}

// Close implements io.Closer.
func (s *simpleReadCloser) Close() error {
	return nil
}
