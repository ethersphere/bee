// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package feeder

import (
	"encoding/binary"

	"github.com/ethersphere/bee/pkg/file/pipeline"
	"github.com/ethersphere/bee/pkg/swarm"
)

const span = swarm.SpanSize

type chunkFeeder struct {
	size      int
	next      pipeline.ChainWriter
	buffer    []byte
	bufferIdx int
	wrote     int64
}

// newChunkFeederWriter creates a new chunkFeeder that allows for partial
// writes into the pipeline. Any pending data in the buffer is flushed to
// subsequent writers when Sum() is called.
func NewChunkFeederWriter(size int, next pipeline.ChainWriter) pipeline.Interface {
	return &chunkFeeder{
		size:   size,
		next:   next,
		buffer: make([]byte, size),
	}
}

// Write writes data to the chunk feeder. It returns the number of bytes written
// to the feeder. The number of bytes written does not necessarily reflect how many
// bytes were actually flushed to subsequent writers, since the feeder is buffered
// and works in chunk-size quantiles.
func (f *chunkFeeder) Write(b []byte) (int, error) {
	l := len(b) // data length
	w := 0      // written

	if l+f.bufferIdx < f.size {
		// write the data into the buffer and return
		n := copy(f.buffer[f.bufferIdx:], b)
		f.bufferIdx += n
		return n, nil
	}

	// if we are here it means we have to do at least one write
	d := make([]byte, f.size+span)
	sp := 0 // span of current write

	//copy from existing buffer to this one
	sp = copy(d[span:], f.buffer[:f.bufferIdx])

	// don't account what was already in the buffer when returning
	// number of written bytes
	if sp > 0 {
		w -= sp
	}

	var n int
	for i := 0; i < len(b); {
		// if we can't fill a whole write, buffer the rest and return
		if sp+len(b[i:]) < f.size {
			n = copy(f.buffer, b[i:])
			f.bufferIdx = n
			return w + n, nil
		}

		// fill stuff up from the incoming write
		n = copy(d[span+f.bufferIdx:], b[i:])
		i += n
		sp += n

		binary.LittleEndian.PutUint64(d[:span], uint64(sp))
		args := &pipeline.PipeWriteArgs{Data: d[:span+sp], Span: d[:span]}
		err := f.next.ChainWrite(args)
		if err != nil {
			return 0, err
		}
		f.bufferIdx = 0
		w += sp
		sp = 0
	}
	f.wrote += int64(w)
	return w, nil
}

// Sum flushes any pending data to subsequent writers and returns
// the cryptographic root-hash respresenting the data written to
// the feeder.
func (f *chunkFeeder) Sum() ([]byte, error) {
	// flush existing data in the buffer
	if f.bufferIdx > 0 {
		d := make([]byte, f.bufferIdx+span)
		copy(d[span:], f.buffer[:f.bufferIdx])
		binary.LittleEndian.PutUint64(d[:span], uint64(f.bufferIdx))
		args := &pipeline.PipeWriteArgs{Data: d, Span: d[:span]}
		err := f.next.ChainWrite(args)
		if err != nil {
			return nil, err
		}
		f.wrote += int64(len(d))
	}

	if f.wrote == 0 {
		// this is an empty file, we should write the span of
		// an empty file (0).
		d := make([]byte, span)
		args := &pipeline.PipeWriteArgs{Data: d, Span: d}
		err := f.next.ChainWrite(args)
		if err != nil {
			return nil, err
		}
		f.wrote += int64(len(d))
	}

	return f.next.Sum()
}
