// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	protocolName  = "hive"
	streamName    = "hive"
	streamVersion = "1.0.0"
)

type Service struct {
	streamer          p2p.Streamer
	logger            logging.Logger
	peerSuggester     PeerSuggester
	saturationTracker SaturationTracker

	tickInterval time.Duration
	done         chan struct{}
	wg           sync.WaitGroup
}

type Options struct {
	Streamer          p2p.Streamer
	Logger            logging.Logger
	PeerSuggester     PeerSuggester
	SaturationTracker SaturationTracker
	TickInterval      time.Duration
}

// PeerSuggester suggests a peer to connect to
type PeerSuggester interface {
	SuggestPeer(addr swarm.Address) (peerAddress swarm.Address, err error)
}

// SaturationTracker tracks weather the saturation has changed
type SaturationTracker interface {
	SaturationChanged() (saturationDepth int, changed bool)
}

func New(o Options) *Service {
	return &Service{
		streamer:          o.Streamer,
		logger:            o.Logger,
		tickInterval:      o.TickInterval,
		peerSuggester:     o.PeerSuggester,
		saturationTracker: o.SaturationTracker,
		done:              make(chan struct{}),
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

func (s *Service) Start() error {
	s.wg.Add(1)
	go func() {
		t := time.NewTicker(s.tickInterval)
		defer t.Stop()
		defer s.wg.Done()

		for {
			select {
			case <-t.C:
				s.hive()
			case <-s.done:
				return
			}
		}
	}()

	return nil
}

func (s *Service) Stop() error {
	close(s.done)
	s.wg.Wait()
	return nil
}

func (s *Service) Handler(peer p2p.Peer, stream p2p.Stream) error {
	return nil
}

func (s *Service) hive() {
	// todo:
}
