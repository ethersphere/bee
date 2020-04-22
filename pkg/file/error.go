// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file

// ErrAborted should be returned whenever a file operation is terminated 
// before it has completed
type ErrAborted struct {
	err error
}

// NewErrAbort creates a new ErrAborted instance
func NewErrAbort(err error) error {
	return &ErrAborted {
		err: err,
	}
}

// Unwrap returns an underlying error
func (e *ErrAborted) Unwrap() error {
	return e.err
}

// Error implement standard go error interface
func (e *ErrAborted) Error() string {
	return e.err.Error()
}
