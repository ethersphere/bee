// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package p2p

// DisconnectError is an error that is specifically handled inside p2p. If returned by specific protocol
// handler it causes peer disconnect.
type DisconnectError struct {
	err error
}

// Disconnect wraps error and creates a special error that is treated specially
// by p2p. It causes peer to disconnect.
func Disconnect(err error) error {
	return &DisconnectError{
		err: err,
	}
}

// Unwrap returns an underlying error.
func (e *DisconnectError) Unwrap() error { return e.err }

// Error implements function of the standard go error interface.
func (e *DisconnectError) Error() string {
	return e.err.Error()
}
