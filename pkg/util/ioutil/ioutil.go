// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ioutil

import (
	"context"
	"io"
	"sync/atomic"
	"time"
)

// idleReader monitors the progress of the io.Reader Read method
// and calls the callbackFn function when reading does not progress
// for the specified idle duration.
type idleReader struct {
	total  atomic.Uint64
	reader io.Reader

	idle  time.Duration
	timer *time.Timer

	done   atomic.Bool
	cancel context.CancelFunc
}

// Read implements the io.Reader Read interface.
func (ir *idleReader) Read(p []byte) (int, error) {
	if !ir.done.Load() && ir.timer.Stop() {
		select {
		case <-ir.timer.C:
		default:
		}
	}

	n, err := ir.reader.Read(p)

	if !ir.done.Load() {
		ir.timer.Reset(ir.idle)
		ir.total.Add(uint64(n))
		if err != nil {
			ir.cancel()
			ir.done.Store(true)
		}
	}
	return n, err
}

// IdleReader creates a new io.Reader instance and starts a goroutine that
// monitors the progress of reading from the given reader. If no progress is
// made for the duration of the given idle, then the reader executes the given
// cancelFn function and terminates the underlying goroutine. This goroutine
// will also terminate when the given context is canceled or if an error is
// returned by the given reader. The duration of the Read method is not taken
// into account when monitoring the progress of reading.
func IdleReader(ctx context.Context, reader io.Reader, idle time.Duration, callbackFn func(uint64)) io.Reader {
	ctx, cancel := context.WithCancel(ctx)
	ir := &idleReader{
		reader: reader,
		timer:  time.NewTimer(idle),
		idle:   idle,
		cancel: cancel,
	}

	go func() {
		select {
		case <-ctx.Done():
		case <-ir.timer.C:
			callbackFn(ir.total.Load())
		}

		ir.done.Store(true)
		ir.timer.Stop()
		select {
		case <-ir.timer.C:
		default:
		}
	}()

	return ir
}

// The WriterFunc type is an adapter to allow the use of
// ordinary functions as io.Writer Write method. If f is
// a function with the appropriate signature, WriterFunc(f)
// is an io.Writer that calls f.
type WriterFunc func([]byte) (int, error)

// WriterFunc calls f(p).
func (f WriterFunc) Write(p []byte) (n int, err error) {
	return f(p)
}
