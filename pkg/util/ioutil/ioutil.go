// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ioutil

import (
	"context"
	"io"
	"time"
)

// timeoutReader monitors the progress of the io.Reader Read method
// and calls the cancel function when reading does not progress for
// the specified time.
type timeoutReader struct {
	r io.Reader
	n chan int
}

// Read implements the io.Reader Read interface.
func (pr *timeoutReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	select {
	case pr.n <- n:
	default:
	}
	return n, err
}

// TimeoutReader creates a new timeoutReader instance and starts
// a goroutine that monitors the progress of reading from the given reader.
// If no progress is made for the duration of the given timeout, then the
// reader executes the given cancel function and terminates the underlying
// goroutine. This goroutine will also terminate when the given context
// is canceled or if the n (returned by the given reader) is equal to 0.
func TimeoutReader(ctx context.Context, r io.Reader, timeout time.Duration, cancel func(uint64)) io.Reader {
	pr := &timeoutReader{r: r, n: make(chan int)}

	go func() {
		defer func() {
			close(pr.n)
			pr.n = nil
		}()

		var (
			total = uint64(0)
			timer = time.NewTimer(timeout)
		)
		for {
			select {
			case n := <-pr.n:
				switch {
				case n == 0:
					return
				case n > 0:
					total += uint64(n)
					timer.Reset(timeout)
				}
			case <-timer.C:
				cancel(total)
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return pr
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
