// Copyright 2019 The Swarm Authors
// This file is part of the Swarm library.
//
// The Swarm library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Swarm library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Swarm library. If not, see <http://www.gnu.org/licenses/>.

package langos

import (
	"bufio"
	"io"
)

// BufferedReadSeeker wraps bufio.Reader to expose Seek method
// from the provided io.ReadSeeker in NewBufferedReadSeeker.
type BufferedReadSeeker struct {
	r  *bufio.Reader
	s  io.ReadSeeker
	ra io.ReaderAt
}

// NewBufferedReadSeeker creates a new instance of BufferedReadSeeker,
// out of io.ReadSeeker. Argument `size` is the size of the read buffer.
func NewBufferedReadSeeker(readSeeker io.ReadSeeker, size int) BufferedReadSeeker {
	ra, _ := readSeeker.(io.ReaderAt)
	return BufferedReadSeeker{
		r:  bufio.NewReaderSize(readSeeker, size),
		s:  readSeeker,
		ra: ra,
	}
}

// Read reads to the byte slice from from buffered reader.
func (b BufferedReadSeeker) Read(p []byte) (n int, err error) {
	return b.r.Read(p)
}

// Seek moves the read position of the underlying ReadSeeker and resets the buffer.
func (b BufferedReadSeeker) Seek(offset int64, whence int) (int64, error) {
	n, err := b.s.Seek(offset, whence)
	b.r.Reset(b.s)
	return n, err
}

// ReadAt implements io.ReaderAt if the provided ReadSeeker also implements it,
// otherwise it returns no error and no bytes read.
func (b BufferedReadSeeker) ReadAt(p []byte, off int64) (n int, err error) {
	if b.ra == nil {
		return 0, nil
	}
	return b.ra.ReadAt(p, off)
}
