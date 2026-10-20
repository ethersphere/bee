// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	ggio "github.com/gogo/protobuf/io"
)

// maxStatusBytes is the largest encoded size of a Status: a root of at most
// 32 prefix bytes, a 32-byte digest, a 32-byte audit digest and five
// varints fit with room to spare.
const maxStatusBytes = 512

// errStreamLimitExceeded is the reason a peer is disconnected for opening too
// many concurrent streams.
var errStreamLimitExceeded = errors.New("rangesync: stream limit exceeded")

// wireMessage is a message read from a peer.
type wireMessage interface {
	protobuf.Message
	Unmarshal([]byte) error
}

// strictMessage decodes a wire message read from a peer, reporting a
// decoding error as a violation (ErrInvalidMessage), unlike the I/O errors of
// the stream. It does not embed m: proto.Unmarshal would then use m's
// XXX_Unmarshal and bypass Unmarshal.
type strictMessage struct {
	m wireMessage
}

func (s strictMessage) Reset()         { s.m.Reset() }
func (s strictMessage) String() string { return s.m.String() }
func (strictMessage) ProtoMessage()    {}

func (s strictMessage) Unmarshal(b []byte) error {
	if err := s.m.Unmarshal(b); err != nil {
		return invalid("undecodable message: %v", err)
	}
	return nil
}

// newReader returns a reader of messages of at most maxSize encoded bytes
// from stream.
func newReader(stream io.Reader, maxSize int) protobuf.Reader {
	return protobuf.Reader{Reader: ggio.NewDelimitedReader(stream, maxSize)}
}

// readMessage reads m from r. A message that does not decode or is longer
// than the reader's limit is a violation: the error wraps ErrInvalidMessage.
// Errors of the stream (io.EOF among them) are returned as they are.
func readMessage(ctx context.Context, r protobuf.Reader, m wireMessage) error {
	err := r.ReadMsgWithContext(ctx, strictMessage{m: m})
	if errors.Is(err, io.ErrShortBuffer) {
		return invalid("message over the size limit")
	}
	return err
}

// violation handles a peer's protocol violation err (wrapping
// ErrInvalidMessage) on stream: it resets the stream and reports the failure
// to the scorer. It returns the error for the handler to return, which makes
// the p2p layer disconnect the peer. rangesync does not blocklist.
func (s *Service) violation(peer swarm.Address, stream p2p.Stream, err error) error {
	_ = stream.Reset()
	s.shortfalls.markViolation(peer, s.now())
	s.metrics.InvalidMessage.Inc()
	s.scorer.Fail(peer, err)
	s.logger.Debug("invalid message from peer, disconnecting", "peer_address", peer, "error", err)
	return p2p.NewDisconnectError(err)
}

// streamLimitExceeded resets the stream and returns an error which makes the
// p2p layer disconnect the peer.
func (s *Service) streamLimitExceeded(peer swarm.Address, stream p2p.Stream, name string, limit int) error {
	_ = stream.Reset()
	s.metrics.StreamLimitExceeded.Inc()
	s.logger.Debug("peer exceeded concurrent stream limit, disconnecting", "peer_address", peer, "stream", name, "limit", limit)
	return p2p.NewDisconnectError(fmt.Errorf("%w: %s stream", errStreamLimitExceeded, name))
}
