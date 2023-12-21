// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// this is a pseudorandom reader that generates a deterministic
// sequence of bytes based on the seed. It is used in tests to
// enable large volumes of pseudorandom data to be generated
// and compared without having to store the data in memory.
package pseudorand

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"

	"github.com/ethersphere/bee/pkg/swarm"
)

const bufSize = 4096

// Reader is a pseudorandom reader that generates a deterministic
// sequence of bytes based on the seed.
type Reader struct {
	cur int
	len int
	seg [40]byte
	buf [bufSize]byte
}

// NewSeed creates a new seed.
func NewSeed() ([]byte, error) {
	seed := make([]byte, 32)
	_, err := io.ReadFull(rand.Reader, seed)
	return seed, err
}

// New creates a new pseudorandom reader seeded with the given seed.
func NewReader(seed []byte, len int) *Reader {
	r := &Reader{len: len}
	_ = copy(r.buf[8:], seed)
	r.fill()
	return r
}

// Read reads len(buf) bytes into buf.	It returns the number of bytes	 read (0 <= n <= len(buf))	and any error encountered.	Even if Read returns n < len(buf), it may use all of buf as scratch space during the call. If some data is available but not len(buf) bytes, Read conventionally returns what is available instead of waiting for more.
func (r *Reader) Read(buf []byte) (n int, err error) {
	toRead := r.len - r.cur
	if toRead < len(buf) {
		buf = buf[:toRead]
	}
	n = copy(buf, r.buf[r.cur%bufSize:])
	r.cur += n
	if r.cur == r.len {
		return n, io.EOF
	}
	if r.cur%bufSize == 0 {
		r.fill()
	}
	return n, nil
}

// Reset resets the reader to the beginning of the stream.
func (r *Reader) Reset() {
	r.Seek(0, 0)
	r.fill()
}

// Equal compares the contents of the reader with the contents of
// the given reader. It returns true if the contents are equal upto n bytes
func (r1 *Reader) Equal(r2 io.Reader) bool {
	r1.Reset()
	buf1 := make([]byte, bufSize)
	buf2 := make([]byte, bufSize)
	read := func(r io.Reader, buf []byte) (n int, err error) {
		for n < len(buf) && err == nil {
			i, e := r.Read(buf[n:])
			if e == nil && i == 0 {
				return n, nil
			}
			err = e
			n += i
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			err = nil
		}
		return n, err
	}
	for {
		n1, err := read(r1, buf1)
		if err != nil {
			return false
		}
		n2, err := read(r2, buf2)
		if err != nil {
			return false
		}
		if n1 < bufSize || n2 < bufSize {
			return n1 == n2 && bytes.Equal(buf1[:n1], buf2[:n2])
		}
		if !bytes.Equal(buf1, buf2) {
			return false
		}
	}
}

// Seek sets the offset for the next Read to offset, interpreted
// according to whence: 0 means relative to the start of the file,
// 1 means relative to the current offset, and 2 means relative to
// the end. It returns the new offset and an error, if any.
func (r *Reader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case 0:
		r.cur = int(offset)
	case 1:
		r.cur += int(offset)
	case 2:
		r.cur = 4096 + int(offset)
	}
	return int64(r.cur), nil
}

// Offset returns the current offset of the reader.
func (r *Reader) Offset() int64 {
	return int64(r.cur)
}

// ReadAt reads len(buf) bytes into buf starting at offset off.
func (r *Reader) ReadAt(buf []byte, off int64) (n int, err error) {
	r.cur = int(off)
	return r.Read(buf)
}

// fill fills the buffer with the hash of the current segment.
func (r *Reader) fill() {
	h := swarm.NewHasher()
	for i := 0; i < len(r.buf); i += 32 {
		binary.BigEndian.PutUint64(r.seg[:], uint64((r.cur+i)/32))
		h.Reset()
		_, _ = h.Write(r.seg[:])
		copy(r.buf[i:i+32], h.Sum(nil))
	}
}
