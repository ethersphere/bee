// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package p2p provides the peer-to-peer abstractions used
// across different protocols in Bee.
package p2p

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/libp2p/go-libp2p/core/network"
	ma "github.com/multiformats/go-multiaddr"
)

// ReachabilityStatus represents the node reachability status.
type ReachabilityStatus network.Reachability

// String implements the fmt.Stringer interface.
func (rs ReachabilityStatus) String() string {
	return network.Reachability(rs).String()
}

const (
	ReachabilityStatusUnknown = ReachabilityStatus(network.ReachabilityUnknown)
	ReachabilityStatusPublic  = ReachabilityStatus(network.ReachabilityPublic)
	ReachabilityStatusPrivate = ReachabilityStatus(network.ReachabilityPrivate)
)

// NetworkStatus represents the network availability status.
type NetworkStatus int

// String implements the fmt.Stringer interface.
func (ns NetworkStatus) String() string {
	str := [...]string{
		NetworkStatusUnknown:     "Unknown",
		NetworkStatusAvailable:   "Available",
		NetworkStatusUnavailable: "Unavailable",
	}
	if ns < 0 || int(ns) >= len(str) {
		return "(unrecognized)"
	}
	return str[ns]
}

const (
	NetworkStatusUnknown     NetworkStatus = 0
	NetworkStatusAvailable   NetworkStatus = 1
	NetworkStatusUnavailable NetworkStatus = 2
)

var ErrNetworkUnavailable = errors.New("network unavailable")

// Service provides methods to handle p2p Peers and Protocols.
type Service interface {
	AddProtocol(ProtocolSpec) error
	// Connect to a peer but do not notify topology about the established connection.
	Connect(ctx context.Context, addr ma.Multiaddr) (address *bzz.Address, err error)
	Disconnecter
	Peers() []Peer
	Blocklisted(swarm.Address) (bool, error)
	BlocklistedPeers() ([]BlockListedPeer, error)
	Addresses() ([]ma.Multiaddr, error)
	SetPickyNotifier(PickyNotifier)
	Halter
	NetworkStatuser
}

// NetworkStatuser handles bookkeeping of the network availability status.
type NetworkStatuser interface {
	// NetworkStatus returns current network availability status.
	NetworkStatus() NetworkStatus
}

type Disconnecter interface {
	Disconnect(overlay swarm.Address, reason string) error
	Blocklister
}

type Blocklister interface {
	NetworkStatuser

	// Blocklist will disconnect a peer and put it on a blocklist (blocking in & out connections) for provided duration
	// Duration 0 is treated as an infinite duration.
	Blocklist(overlay swarm.Address, duration time.Duration, reason string) error
}

type Halter interface {
	// Halt new incoming connections while shutting down
	Halt()
}

// PickyNotifier can decide whether a peer should be picked
type PickyNotifier interface {
	Picker
	Notifier
	ReachabilityUpdater
	ReachableNotifier
}

type Picker interface {
	Pick(Peer) bool
}

type ReachableNotifier interface {
	Reachable(swarm.Address, ReachabilityStatus)
}

type Reacher interface {
	Connected(swarm.Address, ma.Multiaddr)
	Disconnected(swarm.Address)
	Close() error
}

type ReachabilityUpdater interface {
	UpdateReachability(ReachabilityStatus)
}

type Notifier interface {
	Connected(context.Context, Peer, bool) error
	Disconnected(Peer)
	Announce(ctx context.Context, peer swarm.Address, fullnode bool) error
	AnnounceTo(ctx context.Context, addressee, peer swarm.Address, fullnode bool) error
}

// DebugService extends the Service with method used for debugging.
type DebugService interface {
	Service
	SetWelcomeMessage(val string) error
	GetWelcomeMessage() string
}

// Streamer is able to create a new Stream.
type Streamer interface {
	NewStream(ctx context.Context, address swarm.Address, h Headers, protocol, version, stream string) (Stream, error)
}

type StreamerDisconnecter interface {
	Streamer
	Disconnecter
}

// Pinger interface is used to ping a underlay address which is not yet known to the bee node.
// It uses libp2p's default ping protocol. This is different from the PingPong protocol as this
// is meant to be used before we know a particular underlay and we can consider it useful
type Pinger interface {
	Ping(ctx context.Context, addr ma.Multiaddr) (rtt time.Duration, err error)
}

type StreamerPinger interface {
	Streamer
	Pinger
}

// Stream represent a bidirectional data Stream.
type Stream interface {
	io.ReadWriter
	io.Closer
	ResponseHeaders() Headers
	Headers() Headers
	FullClose() error
	Reset() error
}

// ProtocolSpec defines a collection of Stream specifications with handlers.
type ProtocolSpec struct {
	Name          string
	Version       string
	StreamSpecs   []StreamSpec
	ConnectIn     func(context.Context, Peer) error
	ConnectOut    func(context.Context, Peer) error
	DisconnectIn  func(Peer) error
	DisconnectOut func(Peer) error
}

// StreamSpec defines a Stream handling within the protocol.
type StreamSpec struct {
	Name    string
	Handler HandlerFunc
	Headler HeadlerFunc
}

// Peer holds information about a Peer.
type Peer struct {
	Address         swarm.Address
	FullNode        bool
	EthereumAddress []byte
}

// BlockListedPeer holds information about a Peer that is blocked.
type BlockListedPeer struct {
	Peer
	Reason   string
	Duration time.Duration
}

// HandlerFunc handles a received Stream from a Peer.
type HandlerFunc func(context.Context, Peer, Stream) error

// HandlerMiddleware decorates a HandlerFunc by returning a new one.
type HandlerMiddleware func(HandlerFunc) HandlerFunc

// HeadlerFunc is returning response headers based on the received request
// headers.
type HeadlerFunc func(Headers, swarm.Address) Headers

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

type ChunkDeliveryError struct {
	msg string
}

// Error implements the error interface.
func (e *ChunkDeliveryError) Error() string {
	return fmt.Sprintf("delivery of chunk failed: %s", e.msg)
}

// NewChunkDeliveryError is a convenience constructor for ChunkDeliveryError.
func NewChunkDeliveryError(msg string) error {
	return &ChunkDeliveryError{msg: msg}
}
