// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file

import (
	"io"

	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	maxBufferSize = swarm.ChunkSize * 2
)

// ChunkPipe ensures that only the last read is smaller than the chunk size,
// regardless of size of individual writes.
type ChunkPipe struct {
	io.ReadCloser
	writer io.WriteCloser
	data   []byte
	cursor int
}

// Creates a new ChunkPipe
func NewChunkPipe() io.ReadWriteCloser {
	r, w := io.Pipe()
	return &ChunkPipe{
		ReadCloser: r,
		writer:     w,
		data:       make([]byte, maxBufferSize),
	}
}

// Read implements io.Reader
func (c *ChunkPipe) Read(b []byte) (int, error) {
	return c.ReadCloser.Read(b)
}

// Writer implements io.Writer
func (c *ChunkPipe) Write(b []byte) (int, error) {
	nw := 0

	for {
		if nw >= len(b) {
			break
		}

		copied := copy(c.data[c.cursor:], b[nw:])
		c.cursor += copied
		nw += copied

		if c.cursor >= swarm.ChunkSize {
			// NOTE: the Write method contract requires all sent data to be
			// written before returning (without error)
			written, err := c.writer.Write(c.data[:swarm.ChunkSize])
			if err != nil {
				return nw, err
			}
			if swarm.ChunkSize != written {
				return nw, io.ErrShortWrite
			}

			c.cursor -= swarm.ChunkSize

			copy(c.data, c.data[swarm.ChunkSize:])
		}
	}

	return nw, nil
}

// Close implements io.Closer
func (c *ChunkPipe) Close() error {
	if c.cursor > 0 {
		written, err := c.writer.Write(c.data[:c.cursor])
		if err != nil {
			return err
		}
		if c.cursor != written {
			return io.ErrShortWrite
		}
	}
	return c.writer.Close()
}
