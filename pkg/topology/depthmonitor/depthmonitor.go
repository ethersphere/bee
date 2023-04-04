// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package depthmonitor

import (
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	topologyDriver "github.com/ethersphere/bee/pkg/topology"

	"go.uber.org/atomic"
)

const loggerName = "depthmonitor"

// DefaultWakeupInterval is the default value
// for the depth monitor wake-up interval.
const DefaultWakeupInterval = 5 * time.Minute

// defaultMinimumRadius is the default value
// for the depth monitor minimum radius.
const defaultMinimumRadius uint8 = 0

// SyncReporter interface needs to be implemented by the syncing component of the node (puller).
type SyncReporter interface {
	// Number of active historical syncing jobs.
	SyncRate() float64
}

type ReserveReporter interface {
	// ReserveSize returns the current reserve size.
	ReserveSize() uint64
}

// Reserve interface defines the functionality required from the local storage
// of the node to report information about the reserve. The reserve storage is the storage
// pledged by the node to the network.
type Reserve interface {
	// Current size of the reserve.
	ComputeReserveSize(uint8) (uint64, error)
	// Capacity of the reserve that is configured.
	ReserveCapacity() uint64
}

// Topology interface encapsulates the functionality required by the topology component
// of the node.
type Topology interface {
	topologyDriver.SetStorageRadiuser
	topologyDriver.PeersCounter
}

// Service implements the depthmonitor service
type Service struct {
	topology      Topology
	syncer        SyncReporter
	reserve       Reserve
	logger        log.Logger
	bs            postage.Storer
	quit          chan struct{} // to request service to stop
	stopped       chan struct{} // to signal stopping of bg worker
	minimumRadius uint8
	lastRSize     *atomic.Uint64
}

// New constructs a new depthmonitor service
func New(
	t Topology,
	syncer SyncReporter,
	reserve Reserve,
	bs postage.Storer,
	logger log.Logger,
	warmupTime time.Duration,
	wakeupInterval time.Duration,
	freshNode bool,
) *Service {

	s := &Service{
		topology:      t,
		syncer:        syncer,
		reserve:       reserve,
		bs:            bs,
		logger:        logger.WithName(loggerName).Register(),
		quit:          make(chan struct{}),
		stopped:       make(chan struct{}),
		minimumRadius: defaultMinimumRadius,
		lastRSize:     atomic.NewUint64(0),
	}

	go s.manage(warmupTime, wakeupInterval, freshNode)

	return s
}

func (s *Service) manage(warmupTime, wakeupInterval time.Duration, freshNode bool) {
	defer close(s.stopped)

	// wire up batchstore to start reporting storage radius to kademlia
	s.bs.SetStorageRadiusSetter(s.topology)

	// if it's a new fresh node, then we set the storage radius to the reserve radius
	// to prevent syncing from starting at radius zero.
	if freshNode {
		reserveRadius := s.bs.GetReserveState().Radius

		err := s.bs.SetStorageRadius(func(radius uint8) uint8 {
			// if we are starting from scratch, we can use the reserve radius.
			if radius == 0 {
				radius = reserveRadius
			}
			return radius
		})
		if err != nil {
			s.logger.Error(err, "depthmonitor: batchstore set storage radius")
		}
	}

	// wait for warmup
	select {
	case <-s.quit:
		return
	case <-time.After(warmupTime):
	}

	s.logger.Info("depthmonitor: warmup period complete, starting worker", "radius", s.bs.StorageRadius())

	targetSize := s.reserve.ReserveCapacity() * 4 / 10 // 40% of the capacity

	for {
		select {
		case <-s.quit:
			return
		case <-time.After(wakeupInterval):
		}

		radius := s.bs.StorageRadius()

		currentSize, err := s.reserve.ComputeReserveSize(radius)
		if err != nil {
			s.logger.Error(err, "depthmonitor: failed reading reserve size")
			continue
		}

		// save last calculated reserve size
		s.lastRSize.Store(currentSize)

		rate := s.syncer.SyncRate()
		s.logger.Info("depthmonitor: state", "size", currentSize, "radius", radius, "sync_rate", fmt.Sprintf("%.2f ch/s", rate))

		if currentSize > targetSize {
			continue
		}

		// if historical syncing rate is at zero, we proactively decrease the storage radius to allow nodes to widen their neighbourhoods
		if rate == 0 && s.topology.PeersCount(topologyDriver.Filter{}) != 0 {
			err = s.bs.SetStorageRadius(func(radius uint8) uint8 {
				if radius > s.minimumRadius {
					radius--
					s.logger.Info("depthmonitor: reducing storage depth", "depth", radius)
				}
				return radius
			})
			if err != nil {
				s.logger.Error(err, "depthmonitor: batchstore set storage radius")
			}
		}
	}
}

func (s *Service) IsFullySynced() bool {
	return s.syncer.SyncRate() == 0 && s.lastRSize.Load() > s.reserve.ReserveCapacity()*4/10
}

func (s *Service) Close() error {
	close(s.quit)
	select {
	case <-s.stopped:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("stopping depthmonitor with ongoing worker goroutine")
	}
}
