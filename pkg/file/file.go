// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package file provides interfaces for file-oriented operations.
package file

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/ethersphere/bee/pkg/swarm"
)

// JoinSeeker provides a Joiner that can seek.
type JoinSeeker interface {
	Join(ctx context.Context, address swarm.Address) (dataOut io.ReadSeeker, dataLength int64, err error)
	Size(ctx context.Context, address swarm.Address) (dataLength int64, err error)
}

// Splitter starts a new file splitting job.
//
// Data is read from the provided reader.
// If the dataLength parameter is 0, data is read until io.EOF is encountered.
// When EOF is received and splitting is done, the resulting Swarm Address is returned.
type Splitter interface {
	Split(ctx context.Context, dataIn io.ReadCloser, dataLength int64, toEncrypt bool) (addr swarm.Address, err error)
}

// JoinReadAll reads all output from the provided joiner.
func JoinReadAll(ctx context.Context, j JoinSeeker, addr swarm.Address, outFile io.Writer) (int64, error) {
	r, l, err := j.Join(ctx, addr)
	if err != nil {
		return 0, err
	}
	// join, rinse, repeat until done
	data := make([]byte, swarm.ChunkSize)
	var total int64
	for i := int64(0); i < l; i += swarm.ChunkSize {
		cr, err := r.Read(data)
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
