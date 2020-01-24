// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:generate sh -c "protoc -I . -I \"$(go list -f '{{ .Dir }}' -m github.com/gogo/protobuf)/protobuf\" --gogofaster_out=. handshake.proto"

package handshake

import (
	"fmt"
	"io"
	"log"

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
}

func New(overlay string) *Service {
	return &Service{overlay: overlay}
}

func (s *Service) Handshake(stream p2p.Stream) (overlay string, err error) {
	w, r := protobuf.NewWriterAndReader(stream)
	var resp ShakeHand
	if err := w.WriteMsg(&ShakeHand{Address: s.overlay}); err != nil {
		return "", fmt.Errorf("handshake handler: write message: %v\n", err)
	}

	log.Printf("sent handshake req %s\n", s.overlay)
	if err := r.ReadMsg(&resp); err != nil {
		if err == io.EOF {
			return "", nil
		}

		return "", fmt.Errorf("handshake handler: read message: %v\n", err)
	}

	log.Printf("read handshake resp: %s\n", resp.Address)
	return resp.Address, nil
}

func (s *Service) Handler(stream p2p.Stream) string {
	w, r := protobuf.NewWriterAndReader(stream)
	defer stream.Close()

	var req ShakeHand
	if err := r.ReadMsg(&req); err != nil {
		if err == io.EOF {
			return ""
		}
		log.Printf("handshake handler: read message: %v\n", err)
		return ""
	}

	log.Printf("received handshake req %s\n", req.Address)
	if err := w.WriteMsg(&ShakeHand{
		Address: s.overlay,
	}); err != nil {
		log.Printf("handshake handler: write message: %v\n", err)
	}

	log.Printf("sent handshake resp: %s\n", s.overlay)
	return req.Address
}
