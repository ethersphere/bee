// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package p2p

import (
	"context"
	"fmt"
	"io"

	ma "github.com/multiformats/go-multiaddr"
)

type Service interface {
	AddProtocol(ProtocolSpec) error
	Connect(ctx context.Context, addr ma.Multiaddr) (overlay string, err error)
	Disconnect(overlay string) error
}

type Streamer interface {
	NewStream(ctx context.Context, address, protocol, stream, version string) (Stream, error)
}

type Stream interface {
	io.ReadWriter
	io.Closer
}

// PeerSuggester suggests a peer to retrieve a chunk from
type PeerSuggester interface {
	SuggestPeer(addr []byte) (peerAddr string, err error)
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
