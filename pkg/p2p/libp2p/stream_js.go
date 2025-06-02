//go:build js
// +build js

package libp2p

import (
	"errors"
	"io"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
)

type stream struct {
	network.Stream
	headers         map[string][]byte
	responseHeaders map[string][]byte
}

func newStream(s network.Stream) *stream {
	return &stream{Stream: s}
}

func (s *stream) Reset() error {
	return s.Stream.Reset()
}

func (s *stream) FullClose() error {
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
