// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"io"

	"github.com/ethersphere/bee/pkg/swarm"
)

// simpleReadCloser wraps a byte slice in a io.ReadCloser implementation.
type simpleReadCloser struct {
	buffer io.Reader
	closed bool
}

// NewSimpleReadCloser creates a new simpleReadCloser.
func NewSimpleReadCloser(buffer []byte) io.ReadCloser {
	return &simpleReadCloser{
		buffer: bytes.NewBuffer(buffer),
	}
}

// Read implements io.Reader.
func (s *simpleReadCloser) Read(b []byte) (int, error) {
	if s.closed {
		return 0, errors.New("read on closed reader")
	}
	return s.buffer.Read(b)
}

// Close implements io.Closer.
func (s *simpleReadCloser) Close() error {
	if s.closed {
		return errors.New("close on already closed reader")
	}
	s.closed = true
	return nil
}

// JoinReadAll reads all output from the provided Joiner.
func JoinReadAll(ctx context.Context, j Joiner, outFile io.Writer) (int64, error) {
	l := j.Size()

	// join, rinse, repeat until done
	data := make([]byte, swarm.ChunkSize)
	var total int64
	for i := int64(0); i < l; i += swarm.ChunkSize {
		cr, err := j.Read(data)
		if err != nil {
			return total, err
		}
		total += int64(cr)
		cw, err := outFile.Write(data[:cr])
		if err != nil {
			return total, err
		}
		if cw != cr {
			return total, fmt.Errorf("short wrote %d of %d for chunk %d", cw, cr, i)
		}
	}
	if total != l {
		return total, fmt.Errorf("received only %d of %d total bytes", total, l)
	}
	return total, nil
}

// SplitWriteAll writes all input from provided reader to the provided splitter
func SplitWriteAll(ctx context.Context, s Splitter, r io.Reader, l int64, toEncrypt bool) (swarm.Address, error) {
	chunkPipe := NewChunkPipe()
	errC := make(chan error)
	go func() {
		buf := make([]byte, swarm.ChunkSize)
		c, err := io.CopyBuffer(chunkPipe, r, buf)
		if err != nil {
			errC <- err
		}
		if c != l {
			errC <- errors.New("read count mismatch")
		}
		err = chunkPipe.Close()
		if err != nil {
			errC <- err
		}
		close(errC)
	}()

	addr, err := s.Split(ctx, chunkPipe, l, toEncrypt)
	if err != nil {
		return swarm.ZeroAddress, err
	}

	select {
	case err := <-errC:
		if err != nil {
			return swarm.ZeroAddress, err
		}
	case <-ctx.Done():
		return swarm.ZeroAddress, ctx.Err()
	}
	return addr, nil
}

type Loader interface {
	// Load a reference in byte slice representation and return all content associated with the reference.
	Load(context.Context, []byte) ([]byte, error)
}

type Saver interface {
	// Save an arbitrary byte slice and return the reference byte slice representation.
	Save(context.Context, []byte) ([]byte, error)
}

type LoadSaver interface {
	Loader
	Saver
}
