// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package p2p

import (
	"context"
	"io"

	"github.com/ethersphere/bee/pkg/swarm"
	ma "github.com/multiformats/go-multiaddr"
)

type Service interface {
	AddProtocol(ProtocolSpec) error
	Connect(ctx context.Context, addr ma.Multiaddr) (overlay swarm.Address, err error)
	Disconnect(overlay swarm.Address) error
	Peers() []Peer
}

type Streamer interface {
	NewStream(ctx context.Context, address swarm.Address, h Headers, protocol, version, stream string) (Stream, error)
}

type Stream interface {
	io.ReadWriter
	io.Closer
	Headers() Headers
	FullClose() error
}

type ProtocolSpec struct {
	Name        string
	Version     string
	StreamSpecs []StreamSpec
}

type StreamSpec struct {
	Name    string
	Handler HandlerFunc
	Headler HeadlerFunc
}

type Peer struct {
	Address swarm.Address
}

type HandlerFunc func(context.Context, Peer, Stream) error

type HandlerMiddleware func(HandlerFunc) HandlerFunc

type HeadlerFunc func(Headers) Headers

type Headers map[string][]byte

const (
	HeaderNameTracingSpanContext = "tracing-span-context"
)

func NewSwarmStreamName(protocol, version, stream string) string {
	return "/swarm/" + protocol + "/" + version + "/" + stream
}
