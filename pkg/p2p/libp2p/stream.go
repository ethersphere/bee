// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"errors"
	"io"
	"strings"
	"time"

	"github.com/coreos/go-semver/semver"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/libp2p/go-libp2p/core/network"
)

var (
	closeDeadline  = 30 * time.Second
	errExpectedEof = errors.New("read: expected eof")
)
var _ p2p.Stream = (*stream)(nil)

type stream struct {
	network.Stream
	headers         map[string][]byte
	responseHeaders map[string][]byte
	metrics         metrics
}

func newStream(s network.Stream, metrics metrics) *stream {
	return &stream{Stream: s, metrics: metrics}
}

func (s *stream) Headers() p2p.Headers {
	return s.headers
}

func (s *stream) ResponseHeaders() p2p.Headers {
	return s.responseHeaders
}

func (s *stream) Version() (*semver.Version, error) {
	parts := strings.Split(string(s.Stream.Protocol()), "/")
	partsLen := len(parts)
	if partsLen < 2 {
		return nil, errors.New("invalid protocol version")
	}
	return semver.NewVersion(parts[partsLen-2])
}

func (s *stream) Reset() error {
	defer s.metrics.StreamResetCount.Inc()
	return s.Stream.Reset()
}

func (s *stream) FullClose() error {
	defer s.metrics.ClosedStreamCount.Inc()
	// close the stream to make sure it is gc'd
	defer s.Close()

	if err := s.CloseWrite(); err != nil {
		_ = s.Stream.Reset()
		return err
	}

	// So we don't wait forever
	_ = s.SetDeadline(time.Now().Add(closeDeadline))

	// We *have* to observe the EOF. Otherwise, we leak the stream.
	// Now, technically, we should do this *before*
	// returning from SendMessage as the message
	// hasn't really been sent yet until we see the
	// EOF but we don't actually *know* what
	// protocol the other side is speaking.
	n, err := s.Read([]byte{0})
	if n > 0 || err == nil {
		_ = s.Stream.Reset()
		return errExpectedEof
	}
	if !errors.Is(err, io.EOF) {
		_ = s.Stream.Reset()
		return err
	}
	return nil
}
