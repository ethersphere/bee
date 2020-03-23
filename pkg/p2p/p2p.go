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
	Connecter
}

type Streamer interface {
	NewStream(ctx context.Context, address swarm.Address, protocol, version, stream string) (Stream, error)
}

type Connecter interface {
	Connect(ctx context.Context, addr ma.Multiaddr) (overlay swarm.Address, err error)
	Disconnect(overlay swarm.Address) error
	Peers() []Peer
	SetPeerAddedHandler(func(addr swarm.Address))
}

type Stream interface {
	io.ReadWriter
	io.Closer
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
}

type Peer struct {
	Address swarm.Address
}

type HandlerFunc func(Peer, Stream) error

type HandlerMiddleware func(HandlerFunc) HandlerFunc

func NewSwarmStreamName(protocol, version, stream string) string {
	return "/swarm/" + protocol + "/" + version + "/" + stream
}
