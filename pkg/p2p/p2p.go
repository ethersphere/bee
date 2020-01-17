package p2p

import (
	"context"
	"fmt"
	"io"

	ma "github.com/multiformats/go-multiaddr"
)

type Service interface {
	AddProtocol(ProtocolSpec) error
	Connect(ctx context.Context, addr ma.Multiaddr) (peerID string, err error)
	NewStream(ctx context.Context, peerID, protocol, stream, version string) (Stream, error)
}

type Stream interface {
	io.ReadWriter

	// Close closes the stream for writing. Reading will still work (that
	// is, the remote side can still write).
	io.Closer

	// Reset closes both ends of the stream. Use this to tell the remote
	// side to hang up and go away.
	Reset() error

	// Gracefully terminate stream on both ends.
	FullClose() error
}

type Peer struct {
	Addr   ma.Multiaddr
	Stream Stream
}

type ProtocolSpec struct {
	Name        string
	StreamSpecs []StreamSpec
}

type StreamSpec struct {
	Name    string
	Version string
	Handler func(Peer)
}

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
