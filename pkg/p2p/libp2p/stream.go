package libp2p

import (
	"github.com/libp2p/go-libp2p-core/helpers"
	"github.com/libp2p/go-libp2p-core/network"
)

type Stream struct {
	network.Stream
}

func (s *Stream) FullClose() error {
	return helpers.FullClose(s)
}

func NewStream(stream network.Stream) *Stream {
	return &Stream{Stream: stream}
}
