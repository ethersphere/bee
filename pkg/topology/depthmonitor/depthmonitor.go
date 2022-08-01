// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package depthmonitor

import (
	"errors"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	topologyDriver "github.com/ethersphere/bee/pkg/topology"
)

var (
	manageWait = 5 * time.Minute
)

// ReserveReporter interface defines the functionality required from the local storage
// of the node to report information about the reserve. The reserve storage is the storage
// pledged by the node to the network.
type ReserveReporter interface {
	// Current size of the reserve.
	ReserveSize() (uint64, error)
	// Capacity of the reserve that is configured.
	ReserveCapacity() uint64
}

// SyncReporter interface needs to be implemented by the syncing component of the node (pullsync).
type SyncReporter interface {
	// Rate of syncing in terms of chunks/sec.
	Rate() float64
}

// Topology interface encapsulates the functionality required by the topology component
// of the node.
type Topology interface {
	topologyDriver.NeighborhoodDepther
	topologyDriver.SetStorageRadiuser
	topologyDriver.PeersCounter
}

// Service implements the depthmonitor service
type Service struct {
	topology Topology
	syncer   SyncReporter
	reserve  ReserveReporter
	logger   logging.Logger
	bs       postage.Storer
	quit     chan struct{} // to request service to stop
	stopped  chan struct{} // to signal stopping of bg worker
}

// New constructs a new depthmonitor service
func New(
	t Topology,
	syncer SyncReporter,
	reserve ReserveReporter,
	bs postage.Storer,
	logger logging.Logger,
	warmupTime time.Duration,
) *Service {

	s := &Service{
		topology: t,
		syncer:   syncer,
		reserve:  reserve,
		bs:       bs,
		logger:   logger,
		quit:     make(chan struct{}),
		stopped:  make(chan struct{}),
	}

	go s.manage(warmupTime)

	return s
}

func (s *Service) manage(warmupTime time.Duration) {
	defer close(s.stopped)

	// wait for warmup
	select {
	case <-s.quit:
		return
	case <-time.After(warmupTime):
	}

	// wire up batchstore to start reporting storage radius to kademlia
	s.bs.SetStorageRadiusSetter(s.topology)
	reserveRadius := s.bs.GetReserveState().Radius

	err := s.bs.SetStorageRadius(func(radius uint8) uint8 {
		// if we are starting from scratch, we can use the reserve radius.
		if radius == 0 {
			radius = reserveRadius
		}
		s.logger.Infof("depthmonitor: warmup period complete, starting worker with initial depth %d", radius)
		return radius
	})
	if err != nil {
		s.logger.Errorf("depthmonitor: batchstore set storage radius: %w", err)
	}

	halfCapacity := s.reserve.ReserveCapacity() / 2

	for {
		select {
		case <-s.quit:
			return
		case <-time.After(manageWait):
		}

		currentSize, err := s.reserve.ReserveSize()
		if err != nil {
			s.logger.Errorf("depthmonitor: failed reading reserve size %v", err)
			continue
		}

		rate := s.syncer.Rate()
		s.logger.Tracef("depthmonitor: current size %d, %d chunks/sec rate", currentSize, rate)

		// if we have crossed 50% utilization, dont do anything
		if currentSize > halfCapacity {
			continue
		}

		// if historical syncing rate is at zero, we proactively decrease the storage radius to allow nodes to widen their neighbourhoods
		if rate == 0 && s.topology.PeersCount(topologyDriver.Filter{}) != 0 {
			err = s.bs.SetStorageRadius(func(radius uint8) uint8 {
				if radius > 0 {
					radius--
					s.logger.Infof("depthmonitor: reducing storage depth to %d", radius)
				}
				return radius
			})
			if err != nil {
				s.logger.Errorf("depthmonitor: batchstore set storage radius: %w", err)
			}
		}
	}
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
