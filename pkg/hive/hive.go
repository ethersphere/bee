// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
)

const (
	protocolName  = "hive"
	streamName    = "hive"
	streamVersion = "1.0.0"
)

type Service struct {
	streamer p2p.Streamer
	logger   logging.Logger
}

type Options struct {
	Streamer p2p.Streamer
	Logger   logging.Logger
}

func New(o Options) *Service {
	return &Service{
		streamer: o.Streamer,
		logger:   o.Logger,
	}
}

func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name: protocolName,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    streamName,
				Version: streamVersion,
				Handler: s.Handler,
			},
		},
	}
}

func (s *Service) Handler(peer p2p.Peer, stream p2p.Stream) error {
	return nil
}
