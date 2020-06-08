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

type ChunkBuffer struct {
	io.ReadCloser
	writer io.WriteCloser
	data   []byte
	cursor int
}

func NewChunkBuffer() io.ReadWriteCloser {
	r, w := io.Pipe()
	return &ChunkBuffer{
		ReadCloser: r,
		writer:     w,
		data:       make([]byte, maxBufferSize),
	}
}

func (c *ChunkBuffer) Read(b []byte) (int, error) {
	return c.ReadCloser.Read(b)
}

func (c *ChunkBuffer) Write(b []byte) (int, error) {
	copy(c.data[c.cursor:], b)
	c.cursor += len(b)
	if c.cursor >= swarm.ChunkSize {
		c.writer.Write(c.data[:swarm.ChunkSize])
		c.cursor -= swarm.ChunkSize
		copy(c.data, c.data[swarm.ChunkSize:])
	}
	return len(b), nil
}

func (c *ChunkBuffer) Close() error {
	if c.cursor > 0 {
		_, err := c.writer.Write(c.data[:c.cursor])
		if err != nil {
			return err
		}
	}
	c.writer.Close()
	return nil
}
