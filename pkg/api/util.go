// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"io"
	"time"
)

// ProgressReader tracks the progress on io.Reader Read method.
type ProgressReader struct {
	r io.Reader
	n chan int
}

// Read implements the io.Reader Read interface.
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	select {
	case pr.n <- n:
	default:
	}
	return n, err
}

// NewProgressReader creates a new ProgressReader instance and starts
// a goroutine that monitors the progress of reading from the given reader.
// If no progress is made for the duration of the given timeout, then the
// reader executes the given fn function and terminates the underlying
// goroutine. This goroutine will also terminate after the given context
// is canceled or if the n (returned by the given reader) is equal to 0.
func NewProgressReader(ctx context.Context, r io.Reader, timeout time.Duration, fn func(uint64)) *ProgressReader {
	pr := &ProgressReader{r: r, n: make(chan int)}

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
				fn(total)
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return pr
}
