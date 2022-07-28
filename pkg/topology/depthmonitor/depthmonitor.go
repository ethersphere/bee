// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package depthmonitor

import (
	"errors"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/topology"
)

const (
	adaptationFullWindow float64 = 2 * 60 * 60 // seconds allowed to fill half of the fully empty reserve
	adaptationRollback           = 5 * 60      // seconds to slightly roll back the adaption window in case half capacity is not reached
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
	topology.NeighborhoodDepther
	topology.SetStorageRadiuser
}

// Service implements the depthmonitor service
type Service struct {
	syncer  SyncReporter
	reserve ReserveReporter
	logger  logging.Logger
	bs      postage.Storer
	quit    chan struct{} // to request service to stop
	stopped chan struct{} // to signal stopping of bg worker
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
		syncer:  syncer,
		reserve: reserve,
		bs:      bs,
		logger:  logger,
		quit:    make(chan struct{}),
		stopped: make(chan struct{}),
	}

	go s.manage(t, warmupTime)

	return s
}

func (s *Service) manage(topology Topology, warmupTime time.Duration) {
	defer close(s.stopped)

	// wait for warmup
	select {
	case <-s.quit:
		return
	case <-time.After(warmupTime):
	}

	// wire up batchstore to start reporting storage radius to kademlia
	s.bs.SetStorageRadiusSetter(topology)

	err := s.bs.SetStorageRadius(func(radius uint8) uint8 {
		// if we are starting from scratch, we can use the initial NeighborhoodDepth as a starting point.
		if radius == 0 {
			radius = topology.NeighborhoodDepth()
		}
		s.logger.Infof("depthmonitor: warmup period complete, starting worker with initial depth %d", radius)
		return radius
	})
	if err != nil {
		s.logger.Errorf("depthmonitor: batchstore set storage radius: %w", err)
	}

	halfCapacity := float64(s.reserve.ReserveCapacity()) / 2

	var (
		adaptationPeriod bool
		adaptationStart  time.Time                                       // start of the adaptation window
		adaptationWindow float64                                         // allowed time in seconds to fill upto half of the reserve
		adaptationRate   float64   = adaptationFullWindow / halfCapacity // minimum rate of seconds per chunks to fill the reserve
	)

	for {
		select {
		case <-s.quit:
			return
		case <-time.After(manageWait):
		}

		size, err := s.reserve.ReserveSize()
		if err != nil {
			s.logger.Errorf("depthmonitor: failed reading reserve size %v", err)
			continue
		}

		currentSize := float64(size)

		// if we have crossed 50% utilization, dont do anything
		if currentSize > halfCapacity {
			adaptationPeriod = false
			s.logger.Trace("depthmonitor: current size is above half capacity, not entering adaptation period.")
			continue
		}

		// if we dont have 50% utilization of the reserve, enter into an adaptation period
		// to see if we need to modify the depth to improve utilization
		// using a rate of window_size / half_reserve, compute max adaption window time allowed to fill the reserve
		if !adaptationPeriod {
			adaptationPeriod = true
			adaptationStart = time.Now()
			adaptationWindow = adaptationRate * (halfCapacity - currentSize)
			s.logger.Infof("depthmonitor: starting adaptation period with window time %s", time.Second*time.Duration(adaptationWindow))
		}

		// edge case, if we have crossed the adaptation window, roll it back a little to allow sync to fill the reserve
		if time.Since(adaptationStart).Seconds() > adaptationWindow {
			adaptationStart = time.Now().Add(-time.Second * time.Duration(adaptationFullWindow-adaptationRollback))
			s.logger.Infof("depthmonitor: rolling back adaptation window to allow sync to fill reserve")
		}

		// based on the sync rate, determine the expected size of reserve at the end of the
		// adaptation window
		timeleft := adaptationWindow - time.Since(adaptationStart).Seconds()
		expectedSize := s.syncer.Rate()*timeleft + currentSize

		s.logger.Infof(
			"depthmonitor: expected size %.0f with current size %.0f and pullsync rate %.2f ch/s, time left %s",
			expectedSize, currentSize, s.syncer.Rate(), time.Second*time.Duration(timeleft),
		)

		// if we are in the adaptation window and we are not expecting to have enough utilization
		// by the end of it, we proactively decrease the storage depth to allow nodes to widen
		// their neighbourhoods
		if expectedSize < halfCapacity {
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
