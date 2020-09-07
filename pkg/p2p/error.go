// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package p2p

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrPeerNotFound should be returned by p2p service methods when the requested
	// peer is not found.
	ErrPeerNotFound = errors.New("peer not found")
	// ErrAlreadyConnected is returned if connect was called for already connected node.
	ErrAlreadyConnected = errors.New("already connected")
)

// ConnectionBackoffError indicates that connection calls will not be executed until `tryAfter` timetamp.
// The reason is provided in the wrappped error.
type ConnectionBackoffError struct {
	tryAfter time.Time
	err      error
}

// NewConnectionBackoffError creates new `ConnectionBackoffError` with provided underlying error and `tryAfter` timestamp.
func NewConnectionBackoffError(err error, tryAfter time.Time) error {
	return &ConnectionBackoffError{err: err, tryAfter: tryAfter}
}

// TryAfter returns a tryAfter timetamp.
func (e *ConnectionBackoffError) TryAfter() time.Time {
	return e.tryAfter
}

// Unwrap returns an underlying error.
func (e *ConnectionBackoffError) Unwrap() error { return e.err }

// Error implements function of the standard go error interface.
func (e *ConnectionBackoffError) Error() string {
	return e.err.Error()
}

// DisconnectError is an error that is specifically handled inside p2p. If returned by specific protocol
// handler it causes peer disconnect.
type DisconnectError struct {
	err error
}

// NewDisconnectError wraps error and creates a special error that is treated specially
// by p2p. It causes peer to disconnect.
func NewDisconnectError(err error) error {
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

type BlockPeerError struct {
	duration time.Duration
	err      error
}

// NewBlockPeerError wraps error and creates a special error that is treated specially
// by p2p. It causes peer to be disconnected and blocks any new connection for this peer for the provided duration.
func NewBlockPeerError(duration time.Duration, err error) error {
	return &BlockPeerError{
		duration: duration,
		err:      err,
	}
}

// Unwrap returns an underlying error.
func (e *BlockPeerError) Unwrap() error { return e.err }

// Error implements function of the standard go error interface.
func (e *BlockPeerError) Error() string {
	return e.err.Error()
}

// Duration represents the period for which the peer will be blocked.
// 0 duration is treated as infinity
func (e *BlockPeerError) Duration() time.Duration {
	return e.duration
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
