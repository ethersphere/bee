// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package p2p

import (
	"errors"
	"fmt"
)

// ErrPeerNotFound should be returned by p2p service methods when the requested
// peer is not found.
var ErrPeerNotFound = errors.New("peer not found")

// ErrAlreadyConnected is returned if connect was called for already connected node
var ErrAlreadyConnected = errors.New("already connected")

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

// IncompatibleStreamError is the error that should be returned by p2p service
// NewStream method when the stream or its version is not supported.
type IncompatibleStreamError struct {
	err error
}

// NewIncompatibleStreamError wraps the error that is the cause of stream
// incompatibility with IncompatibleStreamError that it can be detected and
// returns it.
func NewIncompatibleStreamError(err error) *IncompatibleStreamError {
	return &IncompatibleStreamError{err: err}
}

// Unwrap returns an underlying error.
func (e *IncompatibleStreamError) Unwrap() error { return e.err }

// Error implements function of the standard go error interface.
func (e *IncompatibleStreamError) Error() string {
	return fmt.Sprintf("incompatible stream: %v", e.err)
}
