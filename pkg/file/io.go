// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package file

import (
	"bytes"

	"io"
)

// simpleJoinerReadCloser wraps a byte slice in a io.ReadCloser implementation.
type simpleReadCloser struct {
	buffer io.Reader
}

func NewSimpleReadCloser(buffer []byte) io.ReadCloser {
	return &simpleReadCloser{
		buffer: bytes.NewBuffer(buffer),
	}
}

// Read implements io.Reader.
func (s *simpleReadCloser) Read(b []byte) (int, error) {
	return s.buffer.Read(b)
}

// Close implements io.Closer.
func (s *simpleReadCloser) Close() error {
	return nil
}
