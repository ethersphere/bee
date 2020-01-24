// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:generate sh -c "protoc -I . -I \"$(go list -f '{{ .Dir }}' -m github.com/gogo/protobuf)/protobuf\" --gogofaster_out=. handshake.proto"

package handshake

import (
	"fmt"

	"github.com/janos/bee/pkg/p2p"
	"github.com/janos/bee/pkg/p2p/protobuf"
)

const (
	ProtocolName  = "handshake"
	StreamName    = "handshake"
	StreamVersion = "1.0.0"
)

type Service struct {
	overlay string
	logger  Logger
}

func New(overlay string, logger Logger) *Service {
	return &Service{
		overlay: overlay,
		logger:  logger,
	}
}

type Logger interface {
	Tracef(format string, args ...interface{})
}

func (s *Service) Handshake(stream p2p.Stream) (overlay string, err error) {
	w, r := protobuf.NewWriterAndReader(stream)
	var resp ShakeHand
	if err := w.WriteMsg(&ShakeHand{Address: s.overlay}); err != nil {
		return "", fmt.Errorf("handshake handler: write message: %w", err)
	}

	s.logger.Tracef("handshake: sent request %s", s.overlay)
	if err := r.ReadMsg(&resp); err != nil {
		return "", fmt.Errorf("handshake handler: read message: %w", err)
	}

	s.logger.Tracef("handshake: read response: %s", resp.Address)
	return resp.Address, nil
}

func (s *Service) Handle(stream p2p.Stream) (overlay string, err error) {
	w, r := protobuf.NewWriterAndReader(stream)
	defer stream.Close()

	var req ShakeHand
	if err := r.ReadMsg(&req); err != nil {
		return "", fmt.Errorf("read message: %w", err)
	}

	s.logger.Tracef("handshake: received request %s", req.Address)
	if err := w.WriteMsg(&ShakeHand{
		Address: s.overlay,
	}); err != nil {
		return "", fmt.Errorf("write message: %w", err)
	}

	s.logger.Tracef("handshake: handled response: %s", s.overlay)
	return req.Address, nil
}
