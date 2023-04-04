// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	Peer          swarm.Address
	Proximity     uint8
	ReserveSize   uint64
	PullsyncRate  float64
	StorageRadius uint8

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

// Snapshots returns the current status snapshot of this node or connected peers.
func (s *Service) Snapshots(ctx context.Context, connectedPeers bool) ([]*Snapshot, error) {
	snapshots := []*Snapshot{{
		ReserveSize:   s.reserve.ReserveSize(),
		PullsyncRate:  s.sync.SyncRate(),
		StorageRadius: s.radius.StorageRadius(),
	}}

	if !connectedPeers {
		return snapshots, nil
	}

	loggerV2 := s.logger.V(2).Register()
	peerFunc := func(address swarm.Address, po uint8) (stop, jumpToNext bool, err error) {
		select {
		case <-ctx.Done():
			return true, false, nil
		default:
		}

		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		ss, err := s.requestStatusSnapshot(ctx, address)
		if err != nil {
			loggerV2.Debug("cannot get status snapshot for peer", "peer_address", address, "error", err)
		}

		snapshots = append(
			snapshots,
			&Snapshot{
				Peer:          address,
				Proximity:     po,
				ReserveSize:   ss.ReserveSize,
				PullsyncRate:  ss.PullsyncRate,
				StorageRadius: uint8(ss.StorageRadius),
				RequestFailed: err != nil,
			})

		return false, false, nil
	}

	err := s.topologyIter.EachConnectedPeer(
		peerFunc,
		topology.Filter{Reachable: true},
	)
	if err != nil {
		s.logger.Error(err, "iteration of connected peers failed")
	}

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

	if err := w.WriteMsgWithContext(ctx, &pb.Snapshot{
		ReserveSize:   s.reserve.ReserveSize(),
		PullsyncRate:  s.sync.SyncRate(),
		StorageRadius: uint32(s.radius.StorageRadius()),
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
