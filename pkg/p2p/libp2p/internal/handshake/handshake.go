// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	"fmt"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake/pb"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
)

const (
	ProtocolName  = "handshake"
	StreamName    = "handshake"
	StreamVersion = "1.0.0"
)

type Service struct {
	overlay   string
	networkID int32
	logger    logging.Logger
}

func New(overlay string, networkID int32, logger logging.Logger) *Service {
	return &Service{
		overlay:   overlay,
		networkID: networkID,
		logger:    logger,
	}
}

func (s *Service) Handshake(stream p2p.Stream) (i *Info, err error) {
	w, r := protobuf.NewWriterAndReader(stream)
	var resp pb.ShakeHand
	if err := w.WriteMsg(&pb.ShakeHand{
		Address:   s.overlay,
		NetworkID: s.networkID,
	}); err != nil {
		return nil, fmt.Errorf("handshake write message: %w", err)
	}

	s.logger.Tracef("handshake sent request %s", s.overlay)
	if err := r.ReadMsg(&resp); err != nil {
		return nil, fmt.Errorf("handshake read message: %w", err)
	}

	s.logger.Tracef("handshake read response: %s", resp.Address)
	return &Info{
		Address:   resp.Address,
		NetworkID: resp.NetworkID,
		Light:     resp.Light,
	}, nil
}

func (s *Service) Handle(stream p2p.Stream) (i *Info, err error) {
	w, r := protobuf.NewWriterAndReader(stream)
	defer stream.Close()

	var req pb.ShakeHand
	if err := r.ReadMsg(&req); err != nil {
		return nil, fmt.Errorf("handshake handler read message: %w", err)
	}

	s.logger.Tracef("handshake handler received request %s", req.Address)
	if err := w.WriteMsg(&pb.ShakeHand{
		Address:   s.overlay,
		NetworkID: s.networkID,
	}); err != nil {
		return nil, fmt.Errorf("handshake handler write message: %w", err)
	}

	s.logger.Tracef("handshake handled response: %s", s.overlay)
	return &Info{
		Address:   req.Address,
		NetworkID: req.NetworkID,
		Light:     req.Light,
	}, nil
}

type Info struct {
	Address   string
	NetworkID int32
	Light     bool
}
