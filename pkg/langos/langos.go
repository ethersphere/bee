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
	"io"
	"sync"
)

// Reader contains all methods that Langos needs to read data from.
type Reader interface {
	io.ReadSeeker
	io.ReaderAt
}

// Langos is a reader with a lookahead peekBuffer
// this is the most naive implementation of a lookahead peekBuffer
// it should issue a lookahead Read when a Read is called, hence
// the name - langos
// |--->====>>------------|
//    cur   topmost
// the first read is not a lookahead but the rest are
// so, it could be that a lookahead read might need to wait for a previous read to finish
// due to resource pooling
//
// All Read and Seek method call must be synchronous.
type Langos struct {
	reader    Reader // reader needs to implement io.ReadSeeker and io.ReaderAt interfaces
	size      int64
	cursor    int64 // current read position
	peeks     []*peek
	peekSize  int
	closed    chan struct{} // terminates peek goroutine and unblocks Read method
	closeOnce sync.Once     // protects closed channel on multiple calls to Close method
}

// NewLangos bakes a new yummy langos that peeks
// on provided reader when its Read method is called.
// Argument peekSize defines the length of peeks.
func NewLangos(r Reader, peekSize int) *Langos {
	return &Langos{
		reader:   r,
		peeks:    make([]*peek, 0),
		peekSize: peekSize,
		closed:   make(chan struct{}),
	}
}

// NewBufferedLangos wraps a new Langos with BufferedReadSeeker
// and returns it.
func NewBufferedLangos(r Reader, bufferSize int) Reader {
	return NewBufferedReadSeeker(NewLangos(r, bufferSize), bufferSize)
}

// Read copies the data to the provided byte slice starting from the
// current read position. The first read will wait for the underlaying
// Reader to return all the data and start a peek on the next data segment.
// All sequential reads will wait for peek to finish reading the data.
// If the current peek is not finished when Read is called, a second peek
// will be started to apprehend the following Read call.
func (l *Langos) Read(p []byte) (n int, err error) {
	pe := l.popPeek(l.cursor)

	// no peek at current cursor
	if pe == nil {
		n, err := l.reader.Read(p)
		if err != nil {
			return n, err
		}
		l.cursor += int64(n)
		// start the peek for the next read
		l.peek(l.cursor)
		return n, err
	}

	select {
	// peek is done, continue to read it
	case <-pe.done:
	default:
		// start the next peek while waiting for the current to finish
		if len(l.peeks) == 0 { // ensure only one second peek
			l.peek(l.cursor + int64(l.peekSize))
		}
	}

	select {
	case <-pe.done:
		bufSize := int64(len(pe.buf))
		// peek detected EOF, store the size if there is none
		if l.size == 0 && pe.err == io.EOF {
			l.size = pe.offset + bufSize
		}

		// peek got an error, return it, but do not pass EOF
		if pe.err != nil && pe.err != io.EOF {
			return 0, pe.err
		}

		// copy peeked data
		start := l.cursor - pe.offset
		n = copy(p, pe.buf[start:])
		// set current cursor
		n64 := int64(n)
		l.cursor += n64
		// preserve buffer tail as another peek
		if l.cursor < pe.offset+bufSize {
			pe.buf = pe.buf[start+n64:]
			pe.offset = l.cursor
			l.addPeek(pe)
		}

		// return EOF if it is reached
		if l.size > 0 && l.cursor >= l.size {
			return n, io.EOF
		}

		// peek from the current cursor
		l.peek(l.cursor)

		return n, nil
	case <-l.closed:
		return 0, io.EOF
	}
}

// Seek moves the Read cursor to a specific position.
func (l *Langos) Seek(offset int64, whence int) (int64, error) {
	n, err := l.reader.Seek(offset, whence)
	if err != nil {
		return n, err
	}
	// seek got data size, store it
	if whence == io.SeekEnd {
		l.size = n
	}
	l.cursor = n
	return n, err
}

// ReadAt reads the data on offset and does not add any optimizations.
func (l *Langos) ReadAt(p []byte, off int64) (int, error) {
	return l.reader.ReadAt(p, off)
}

// Close unblocks Read method calls that are waiting for peek to finish.
func (l *Langos) Close() (err error) {
	l.closeOnce.Do(func() {
		close(l.closed)
	})
	return nil
}

// peek starts a new peek ad offset with peekSize data length. The peek
// can be retrieved by popPeek Langos method.
func (l *Langos) peek(offset int64) {
	// if here already is a peek that
	// contains data at this offset,
	// do not create another one
	if l.hasPeek(offset) {
		return
	}

	p := &peek{
		offset: offset,
		done:   make(chan struct{}),
		buf:    make([]byte, l.peekSize),
	}
	l.addPeek(p)

	// start a goroutine to peek the data
	go func() {
		n, err := l.reader.ReadAt(p.buf, offset)

		if n >= 0 && n < len(p.buf) { // protect from invalid n (from lazy chunk reader)
			p.mu.Lock()
			p.buf = p.buf[:n]
			p.mu.Unlock()
		}
		p.err = err
		close(p.done)
	}()
}

func (l *Langos) addPeek(p *peek) {
	l.peeks = append(l.peeks, p)
}

// popPeek returns a peek that includes the offset and removes it
// from langos. Nil is returned if there is no peek.
func (l *Langos) popPeek(offset int64) (p *peek) {
	for i, p := range l.peeks {
		if p.has(offset) {
			l.peeks = append(l.peeks[:i], l.peeks[i+1:]...)
			return p
		}
	}
	return nil
}

// hasPeek returns true if there is a peek that includes the given offset.
func (l *Langos) hasPeek(offset int64) (yes bool) {
	for _, p := range l.peeks {
		if p.has(offset) {
			return true
		}
	}
	return false
}

// peek holds the current state of a read at some offset. When the read is done,
// done channel is closed and buffer is safe to read up to the size if error is not nil.
type peek struct {
	offset int64         // peek cursor position
	buf    []byte        // peeked data
	mu     sync.RWMutex  // protects buf length change and len read in has method
	err    error         // error returned by ReadAt on peeking
	done   chan struct{} // closed when the peek is done so that Read can copy buf data
}

// has returns whether the peek has, or should have after it is done,
// data starting from the offset.
func (p *peek) has(offset int64) (yes bool) {
	// peek offset does not start from required offset
	if offset < p.offset {
		return false
	}
	p.mu.RLock()
	bufSize := int64(len(p.buf))
	p.mu.RUnlock()
	return offset < p.offset+bufSize
}
