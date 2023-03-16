// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ioutil

import (
	"io"
	"sync"
	"time"
)

// timeoutReader monitors the progress of the io.Reader Read method
// and calls the cancel function when reading does not progress for
// the specified time.
type timeoutReader struct {
	reader io.Reader
	resC   chan readResult
	lock   sync.Mutex
}

type readResult struct {
	n   int
	err error
}

// Read implements the io.Reader interface.
func (tr *timeoutReader) Read(p []byte) (int, error) {
	n, err := tr.reader.Read(p)

	tr.lock.Lock()
	resC := tr.resC
	tr.lock.Unlock()

	if resC != nil {
		resC <- readResult{n, err}
	}

	return n, err
}

// TimeoutReader creates a new timeoutReader instance and starts
// a goroutine that monitors the progress of reading from the given reader.
// If no progress is made for the duration of the given timeout, then the
// reader executes the given callback function and terminates the underlying
// goroutine. This goroutine will also terminate when the given context
// is canceled or if the n (returned by the given reader) is equal to 0.
func TimeoutReader(r io.Reader, timeout time.Duration, callback func(uint64)) io.Reader {
	tr := &timeoutReader{reader: r, resC: make(chan readResult)}

	go func() {
		var (
			total = uint64(0)
			timer = time.NewTimer(timeout)
		)

		cleanup := func() {
			timer.Stop()

			tr.lock.Lock()
			tr.resC = nil
			tr.lock.Unlock()
		}

		for {
			select {
			case result := <-tr.resC:
				total += uint64(result.n)
				timer.Reset(timeout)

				if result.err != nil {
					cleanup()
					return
				}
			case <-timer.C:
				cleanup()
				callback(total)
				return
			}
		}
	}()

	return tr
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
