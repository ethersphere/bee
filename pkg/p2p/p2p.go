// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package p2p

import (
	"context"
	"fmt"
	"io"

	"github.com/ethersphere/bee/pkg/swarm"
	ma "github.com/multiformats/go-multiaddr"
)

type Service interface {
	AddProtocol(ProtocolSpec) error
	Connect(ctx context.Context, addr ma.Multiaddr) (overlay swarm.Address, err error)
	Disconnect(overlay swarm.Address) error
}

type Streamer interface {
	NewStream(ctx context.Context, address swarm.Address, protocol, stream, version string) (Stream, error)
}

type Stream interface {
	io.ReadWriter
	io.Closer
}

type ProtocolSpec struct {
	Name        string
	StreamSpecs []StreamSpec
}

type StreamSpec struct {
	Name    string
	Version string
	Handler HandlerFunc
}

type HandlerFunc func(Peer, Stream) error

type HandlerMiddleware func(HandlerFunc) HandlerFunc

type IncompatibleStreamError struct {
	err error
}

func NewIncompatibleStreamError(err error) *IncompatibleStreamError {
	return &IncompatibleStreamError{err: err}
}

func (e *IncompatibleStreamError) Unwrap() error { return e.err }

func (e *IncompatibleStreamError) Error() string {
	return fmt.Sprintf("incompatible stream: %v", e.err)
}

func NewSwarmStreamName(protocol, stream, version string) string {
	return "/swarm/" + protocol + "/" + stream + "/" + version
}
