// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"bytes"

	"github.com/ethersphere/bee/pkg/p2p"
)

type Stream struct {
	readBuffer        *bytes.Buffer
	writeBuffer       *bytes.Buffer
	writeCounter      int
	readCounter       int
	readError         error
	writeError        error
	readErrCheckmark  int
	writeErrCheckmark int
}

func NewStream(readBuffer, writeBuffer *bytes.Buffer) *Stream {
	return &Stream{readBuffer: readBuffer, writeBuffer: writeBuffer}
}
func (s *Stream) SetReadErr(err error, checkmark int) {
	s.readError = err
	s.readErrCheckmark = checkmark
}

func (s *Stream) SetWriteErr(err error, checkmark int) {
	s.writeError = err
	s.writeErrCheckmark = checkmark
}

func (s *Stream) Read(p []byte) (n int, err error) {
	if s.readError != nil && s.readErrCheckmark <= s.readCounter {
		return 0, s.readError
	}

	s.readCounter++
	return s.readBuffer.Read(p)
}

func (s *Stream) Write(p []byte) (n int, err error) {
	if s.writeError != nil && s.writeErrCheckmark <= s.writeCounter {
		return 0, s.writeError
	}

	s.writeCounter++
	return s.writeBuffer.Write(p)
}

func (s *Stream) Headers() p2p.Headers {
	return nil
}

func (s *Stream) ResponseHeaders() p2p.Headers {
	return nil
}

func (s *Stream) Close() error {
	return nil
}

func (s *Stream) FullClose() error {
	return nil
}

func (s *Stream) Reset() error {
	return nil
}
