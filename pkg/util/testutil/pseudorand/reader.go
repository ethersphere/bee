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
	"fmt"
	"io"

	"github.com/ethersphere/bee/v2/pkg/swarm"
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
func NewReader(seed []byte, l int) *Reader {
	r := &Reader{len: l}
	_ = copy(r.buf[8:], seed)
	r.fill()
	return r
}

// Size returns the size of the reader.
func (r *Reader) Size() int {
	return r.len
}

// Read reads len(buf) bytes into buf.	It returns the number of bytes	 read (0 <= n <= len(buf))	and any error encountered.	Even if Read returns n < len(buf), it may use all of buf as scratch space during the call. If some data is available but not len(buf) bytes, Read conventionally returns what is available instead of waiting for more.
func (r *Reader) Read(buf []byte) (n int, err error) {
	cur := r.cur % bufSize
	toRead := min(bufSize-cur, r.len-r.cur)
	if toRead < len(buf) {
		buf = buf[:toRead]
	}
	n = copy(buf, r.buf[cur:])
	r.cur += n
	if r.cur == r.len {
		return n, io.EOF
	}
	if r.cur%bufSize == 0 {
		r.fill()
	}
	return n, nil
}

// Equal compares the contents of the reader with the contents of
// the given reader. It returns true if the contents are equal upto n bytes
func (r1 *Reader) Equal(r2 io.Reader) (bool, error) {
	ok, err := r1.Match(r2, r1.len)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	n, err := io.ReadFull(r2, make([]byte, 1))
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return n == 0, nil
	}
	return false, err
}

// Match compares the contents of the reader with the contents of
// the given reader. It returns true if the contents are equal upto n bytes
func (r1 *Reader) Match(r2 io.Reader, l int) (bool, error) {

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

	buf1 := make([]byte, bufSize)
	buf2 := make([]byte, bufSize)
	for l > 0 {
		if l <= len(buf1) {
			buf1 = buf1[:l]
			buf2 = buf2[:l]
		}

		n1, err := read(r1, buf1)
		if err != nil {
			return false, err
		}
		n2, err := read(r2, buf2)
		if err != nil {
			return false, err
		}
		if n1 != n2 {
			return false, nil
		}
		if !bytes.Equal(buf1[:n1], buf2[:n2]) {
			return false, nil
		}
		l -= n1
	}
	return true, nil
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
		r.cur = r.len - int(offset)
	}
	if r.cur < 0 || r.cur > r.len {
		return 0, fmt.Errorf("seek: invalid offset %d", r.cur)
	}
	r.fill()
	return int64(r.cur), nil
}

// Offset returns the current offset of the reader.
func (r *Reader) Offset() int64 {
	return int64(r.cur)
}

// ReadAt reads len(buf) bytes into buf starting at offset off.
func (r *Reader) ReadAt(buf []byte, off int64) (n int, err error) {
	if _, err := r.Seek(off, io.SeekStart); err != nil {
		return 0, err
	}
	return r.Read(buf)
}

// fill fills the buffer with the hash of the current segment.
func (r *Reader) fill() {
	if r.cur >= r.len {
		return
	}
	bufSegments := bufSize / 32
	start := r.cur / bufSegments
	rem := (r.cur % bufSize) / 32
	h := swarm.NewHasher()
	for i := 32 * rem; i < len(r.buf); i += 32 {
		binary.BigEndian.PutUint64(r.seg[:], uint64((start+i)/32))
		h.Reset()
		_, _ = h.Write(r.seg[:])
		copy(r.buf[i:], h.Sum(nil))
	}
}
