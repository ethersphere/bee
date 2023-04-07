// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/status/internal/pb"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/topology/depthmonitor"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "status"

const (
	protocolName    = "status"
	protocolVersion = "1.0.0"
	streamName      = "status"
)

// Snapshot is the current snapshot of the system.
type Snapshot struct {
	Peer             swarm.Address
	Proximity        uint8
	ReserveSize      uint64
	PullsyncRate     float64
	StorageRadius    uint8
	ConnectedPeers   uint64
	NeighborhoodSize uint64

	RequestFailed bool // Indicates whether there was an error while requesting the snapshot.
}

// Service is the status service.
type Service struct {
	logger       log.Logger
	streamer     p2p.Streamer
	topologyIter topology.PeerIterator

	reserve depthmonitor.ReserveReporter
	sync    depthmonitor.SyncReporter
	radius  postage.RadiusReporter
}

// LocalSnapshot returns the current status snapshot of this node.
func (s *Service) LocalSnapshot() *Snapshot {
	return &Snapshot{
		ReserveSize:   s.reserve.ReserveSize(),
		PullsyncRate:  s.sync.SyncRate(),
		StorageRadius: s.radius.StorageRadius(),
	}
}

// ConnectedPeersSnapshot returns the current status snapshot of this node connected peers.
func (s *Service) ConnectedPeersSnapshot(ctx context.Context) ([]*Snapshot, error) {
	var snapshots []*Snapshot

	var (
		wg  sync.WaitGroup
		mtx sync.Mutex
	)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	peerFunc := func(address swarm.Address, po uint8) (bool, bool, error) {
		wg.Add(1)
		go func() {
			defer wg.Done()

			snapshot := &Snapshot{Peer: address, Proximity: po}

			ss, err := s.requestStatusSnapshot(ctx, address)
			if err != nil {
				s.logger.Debug("cannot get status snapshot for peer", "peer_address", address, "error", err)
				snapshot.RequestFailed = true
			} else {
				snapshot.ReserveSize = ss.ReserveSize
				snapshot.PullsyncRate = ss.PullsyncRate
				snapshot.StorageRadius = uint8(ss.StorageRadius)
				snapshot.ConnectedPeers = ss.ConnectedPeers
				snapshot.NeighborhoodSize = ss.NeighborhoodSize
			}

			mtx.Lock()
			snapshots = append(snapshots, snapshot)
			mtx.Unlock()
		}()

		return false, false, nil
	}

	err := s.topologyIter.EachConnectedPeer(
		peerFunc,
		topology.Filter{Reachable: true},
	)
	if err != nil {
		s.logger.Error(err, "iteration of connected peers failed")
	}

	wg.Wait()

	return snapshots, nil
}

// Protocol returns the protocol specification.
func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    protocolName,
		Version: protocolVersion,
		StreamSpecs: []p2p.StreamSpec{{
			Name:    streamName,
			Handler: s.handler,
		}},
	}
}

// requestStatusSnapshot sends request for status snapshot to the peer.
func (s *Service) requestStatusSnapshot(ctx context.Context, peer swarm.Address) (*pb.Snapshot, error) {
	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return nil, fmt.Errorf("new stream: %w", err)
	}
	defer func() {
		go stream.FullClose()
	}()

	w, r := protobuf.NewWriterAndReader(stream)

	if err := w.WriteMsgWithContext(ctx, new(pb.Get)); err != nil {
		return nil, fmt.Errorf("write message failed: %w", err)
	}

	ss := new(pb.Snapshot)
	if err := r.ReadMsgWithContext(ctx, ss); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, fmt.Errorf("read message failed: %w", err)
	}
	return ss, nil
}

// handler handles the status stream request/response.
func (s *Service) handler(ctx context.Context, _ p2p.Peer, stream p2p.Stream) error {
	loggerV2 := s.logger.V(2).Register()

	w, r := protobuf.NewWriterAndReader(stream)
	defer func() {
		if err := stream.FullClose(); err != nil {
			loggerV2.Debug("stream full close failed: %v", "error", err)
		}
	}()

	var msgGet pb.Get
	if err := r.ReadMsgWithContext(ctx, &msgGet); err != nil && !errors.Is(err, io.EOF) {
		loggerV2.Debug("read message failed", "error", err)
		return fmt.Errorf("read message: %w", err)
	}

	var (
		storageRadius    = s.radius.StorageRadius()
		connectedPeers   uint64
		neighborhoodSize uint64
	)
	err := s.topologyIter.EachConnectedPeer(
		func(_ swarm.Address, po uint8) (bool, bool, error) {
			connectedPeers++
			if po >= storageRadius {
				neighborhoodSize++
			}
			return false, false, nil
		},
		topology.Filter{Reachable: true},
	)
	if err != nil {
		s.logger.Error(err, "iteration of connected peers failed")
		return fmt.Errorf("iterate connected peers: %w", err)
	}

	if err := w.WriteMsgWithContext(ctx, &pb.Snapshot{
		ReserveSize:      s.reserve.ReserveSize(),
		PullsyncRate:     s.sync.SyncRate(),
		StorageRadius:    uint32(storageRadius),
		ConnectedPeers:   connectedPeers,
		NeighborhoodSize: neighborhoodSize,
	}); err != nil {
		loggerV2.Debug("write message failed", "error", err)
		return fmt.Errorf("write message: %w", err)
	}

	return nil
}

// NewService creates a new status service.
func NewService(
	logger log.Logger,
	streamer p2p.Streamer,
	topologyIter topology.PeerIterator,
	reserve depthmonitor.ReserveReporter,
	sync depthmonitor.SyncReporter,
	radius postage.RadiusReporter,
) *Service {
	return &Service{
		logger:       logger.WithName(loggerName).Register(),
		streamer:     streamer,
		topologyIter: topologyIter,
		reserve:      reserve,
		sync:         sync,
		radius:       radius,
	}
}
