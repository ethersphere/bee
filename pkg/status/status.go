// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status

import (
	"context"
	"fmt"

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
	protocolVersion = "1.1.0"
	streamName      = "status"
)

// Snapshot is the current snapshot of the system.
type Snapshot pb.Snapshot

type topologyDriver interface {
	topology.PeerIterator
	IsReachable() bool
}

// Service is the status service.
type Service struct {
	logger         log.Logger
	streamer       p2p.Streamer
	topologyDriver topologyDriver

	beeMode    string
	reserve    depthmonitor.ReserveReporter
	sync       depthmonitor.SyncReporter
	radius     postage.RadiusReporter
	commitment postage.CommitmentGetter
}

// LocalSnapshot returns the current status snapshot of this node.
func (s *Service) LocalSnapshot() (*Snapshot, error) {
	var (
		storageRadius    = s.radius.StorageRadius()
		connectedPeers   uint64
		neighborhoodSize uint64
	)
	err := s.topologyDriver.EachConnectedPeer(
		func(_ swarm.Address, po uint8) (bool, bool, error) {
			connectedPeers++
			if po >= storageRadius {
				neighborhoodSize++
			}
			return false, false, nil
		},
		topology.Filter{},
	)
	if err != nil {
		return nil, fmt.Errorf("iterate connected peers: %w", err)
	}

	commitment, err := s.commitment.Commitment()
	if err != nil {
		return nil, fmt.Errorf("batchstore commitment: %w", err)
	}

	return &Snapshot{
		BeeMode:          s.beeMode,
		ReserveSize:      s.reserve.ReserveSize(),
		PullsyncRate:     s.sync.SyncRate(),
		StorageRadius:    uint32(storageRadius),
		ConnectedPeers:   connectedPeers,
		NeighborhoodSize: neighborhoodSize,
		BatchCommitment:  commitment,
		IsReachable:      s.topologyDriver.IsReachable(),
	}, nil
}

// PeerSnapshot sends request for status snapshot to the peer.
func (s *Service) PeerSnapshot(ctx context.Context, peer swarm.Address) (*Snapshot, error) {
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
		return nil, fmt.Errorf("read message failed: %w", err)
	}

	return (*Snapshot)(ss), nil
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
	if err := r.ReadMsgWithContext(ctx, &msgGet); err != nil {
		loggerV2.Debug("read message failed", "error", err)
		return fmt.Errorf("read message: %w", err)
	}

	var (
		storageRadius    = s.radius.StorageRadius()
		connectedPeers   uint64
		neighborhoodSize uint64
	)
	err := s.topologyDriver.EachConnectedPeer(
		func(_ swarm.Address, po uint8) (bool, bool, error) {
			connectedPeers++
			if po >= storageRadius {
				neighborhoodSize++
			}
			return false, false, nil
		},
		topology.Filter{},
	)
	if err != nil {
		s.logger.Error(err, "iteration of connected peers failed")
		return fmt.Errorf("iterate connected peers: %w", err)
	}

	commitment, err := s.commitment.Commitment()
	if err != nil {
		return fmt.Errorf("batchstore commitemnt: %w", err)
	}

	if err := w.WriteMsgWithContext(ctx, &pb.Snapshot{
		BeeMode:          s.beeMode,
		ReserveSize:      s.reserve.ReserveSize(),
		PullsyncRate:     s.sync.SyncRate(),
		StorageRadius:    uint32(storageRadius),
		ConnectedPeers:   connectedPeers,
		NeighborhoodSize: neighborhoodSize,
		BatchCommitment:  commitment,
		IsReachable:      s.topologyDriver.IsReachable(),
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
	topology topologyDriver,
	beeMode string,
	reserve depthmonitor.ReserveReporter,
	sync depthmonitor.SyncReporter,
	radius postage.RadiusReporter,
	commitment postage.CommitmentGetter,
) *Service {
	return &Service{
		logger:         logger.WithName(loggerName).Register(),
		streamer:       streamer,
		topologyDriver: topology,
		beeMode:        beeMode,
		reserve:        reserve,
		sync:           sync,
		radius:         radius,
		commitment:     commitment,
	}
}
