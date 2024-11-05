// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status

import (
	"context"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/status/internal/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "status"

const (
	protocolName    = "status"
	protocolVersion = "1.1.2"
	streamName      = "status"
)

// Snapshot is the current snapshot of the system.
type Snapshot pb.Snapshot

// SyncReporter defines the interface to report syncing rate.
type SyncReporter interface {
	SyncRate() float64
}

// Reserve defines the reserve storage related information required.
type Reserve interface {
	ReserveSize() int
	ReserveSizeWithinRadius() uint64
	StorageRadius() uint8
	CommittedDepth() uint8
}

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
	reserve    Reserve
	sync       SyncReporter
	chainState postage.ChainStateGetter
}

// NewService creates a new status service.
func NewService(
	logger log.Logger,
	streamer p2p.Streamer,
	topology topologyDriver,
	beeMode string,
	chainState postage.ChainStateGetter,
	reserve Reserve,
) *Service {
	return &Service{
		logger:         logger.WithName(loggerName).Register(),
		streamer:       streamer,
		topologyDriver: topology,
		beeMode:        beeMode,
		chainState:     chainState,
		reserve:        reserve,
	}
}

// LocalSnapshot returns the current status snapshot of this node.
func (s *Service) LocalSnapshot() (*Snapshot, error) {
	var (
		storageRadius           uint8
		syncRate                float64
		reserveSize             uint64
		reserveSizeWithinRadius uint64
		connectedPeers          uint64
		neighborhoodSize        uint64
		committedDepth          uint8
	)

	if s.reserve != nil {
		storageRadius = s.reserve.StorageRadius()
		reserveSize = uint64(s.reserve.ReserveSize())
		reserveSizeWithinRadius = s.reserve.ReserveSizeWithinRadius()
		committedDepth = s.reserve.CommittedDepth()
	}

	if s.sync != nil {
		syncRate = s.sync.SyncRate()
	}

	commitment, err := s.chainState.Commitment()
	if err != nil {
		return nil, fmt.Errorf("batchstore commitment: %w", err)
	}

	err = s.topologyDriver.EachConnectedPeer(
		func(_ swarm.Address, po uint8) (bool, bool, error) {
			connectedPeers++
			if po >= storageRadius {
				neighborhoodSize++
			}
			return false, false, nil
		},
		topology.Select{},
	)
	if err != nil {
		return nil, fmt.Errorf("iterate connected peers: %w", err)
	}

	return &Snapshot{
		BeeMode:                 s.beeMode,
		ReserveSize:             reserveSize,
		ReserveSizeWithinRadius: reserveSizeWithinRadius,
		PullsyncRate:            syncRate,
		StorageRadius:           uint32(storageRadius),
		ConnectedPeers:          connectedPeers,
		NeighborhoodSize:        neighborhoodSize + 1, // include self
		BatchCommitment:         commitment,
		IsReachable:             s.topologyDriver.IsReachable(),
		LastSyncedBlock:         s.chainState.GetChainState().Block,
		CommittedDepth:          uint32(committedDepth),
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

	snapshot, err := s.LocalSnapshot()
	if err != nil {
		loggerV2.Debug("local snapshot failed", "error", err)
		return fmt.Errorf("local snapshot: %w", err)
	}

	if err := w.WriteMsgWithContext(ctx, (*pb.Snapshot)(snapshot)); err != nil {
		loggerV2.Debug("write message failed", "error", err)
		return fmt.Errorf("write message: %w", err)
	}

	return nil
}

func (s *Service) SetSync(sync SyncReporter) {
	s.sync = sync
}
