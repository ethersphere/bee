// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package safe

import (
	"fmt"
	"runtime/debug"

	"github.com/ethersphere/bee/v2/pkg/log"
)

// Go runs the given function in a new goroutine and recovers from panic in it.
// Panics are logged using the provided logger (if non-nil).
// If the logger is nil, the function is run without panic recovery.
func Go(logger log.Logger, name string, fn func()) {
	go func() {
		if logger != nil {
			defer func() {
				if r := recover(); r != nil {
					logger.Error(nil, "goroutine panic recovered", "name", name, "panic", fmt.Sprintf("%v", r), "stack", string(debug.Stack()))
				}
			}()
		}
		fn()
	}()
}

// Run runs the given function synchronously and recovers from panic in it.
// Panics are logged using the provided logger (if non-nil).
// If the logger is nil, the function is run without panic recovery.
func Run(logger log.Logger, name string, fn func()) {
	if logger != nil {
		defer func() {
			if r := recover(); r != nil {
				logger.Error(nil, "panic recovered", "name", name, "panic", fmt.Sprintf("%v", r), "stack", string(debug.Stack()))
			}
		}()
	}
	fn()
}

// RunFunc returns a function wrapped with panic recovery, suitable for use in errgroup.Go.
// Panics are logged using the provided logger (if non-nil), and the returned function returns a non-nil error.
// If the recovered value implements error, it is wrapped using %w to preserve error type checking.
func RunFunc(logger log.Logger, name string, fn func() error) func() error {
	return func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				if logger != nil {
					logger.Error(nil, "errgroup goroutine panic recovered", "name", name, "panic", fmt.Sprintf("%v", r), "stack", string(debug.Stack()))
				}
				if panicErr, ok := r.(error); ok {
					err = fmt.Errorf("panic in %s: %w", name, panicErr)
				} else {
					err = fmt.Errorf("panic in %s: %v", name, r)
				}
			}
		}()
		return fn()
	}
}
