// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package p2p

import (
	"context"
	"io"

	"github.com/ethersphere/bee/pkg/bzz"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	ma "github.com/multiformats/go-multiaddr"
)

// Service provides methods to handle p2p Peers and Protocols.
type Service interface {
	AddProtocol(ProtocolSpec) error
	Connect(ctx context.Context, addr ma.Multiaddr, notify bool) (address *bzz.Address, err error)
	Disconnect(overlay swarm.Address) error
	Peers() []Peer
	SetNotifier(topology.Notifier)
	Addresses() ([]ma.Multiaddr, error)
}

// Streamer is able to create a new Stream.
type Streamer interface {
	NewStream(ctx context.Context, address swarm.Address, h Headers, protocol, version, stream string) (Stream, error)
}

// Stream represent a bidirectional data Stream.
type Stream interface {
	io.ReadWriter
	io.Closer
	Headers() Headers
	FullClose() error
}

// ProtocolSpec defines a collection of Stream specifications with handlers.
type ProtocolSpec struct {
	Name        string
	Version     string
	StreamSpecs []StreamSpec
}

// StreamSpec defines a Stream handling within the protocol.
type StreamSpec struct {
	Name    string
	Handler HandlerFunc
	Headler HeadlerFunc
}

// Peer holds information about a Peer.
type Peer struct {
	Address swarm.Address `json:"address"`
}

// HandlerFunc handles a received Stream from a Peer.
type HandlerFunc func(context.Context, Peer, Stream) error

// HandlerMiddleware decorates a HandlerFunc by returning a new one.
type HandlerMiddleware func(HandlerFunc) HandlerFunc

// HeadlerFunc is returning response headers based on the received request
// headers.
type HeadlerFunc func(Headers) Headers

// Headers represents a collection of p2p header key value pairs.
type Headers map[string][]byte

// Common header names.
const (
	HeaderNameTracingSpanContext = "tracing-span-context"
)

// NewSwarmStreamName constructs a libp2p compatible stream name out of
// protocol name and version and stream name.
func NewSwarmStreamName(protocol, version, stream string) string {
	return "/swarm/" + protocol + "/" + version + "/" + stream
}
