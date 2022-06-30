package depthmonitor

import (
	"errors"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/topology"
)

const (
	depthKey                  string  = "storage_depth"
	adaptationWindowSeconds   float64 = 2 * 60 * 60 // 2 hours to fill half the empty reserve
	adaptationRollbackMinutes         = 5
	manageWait                        = 5 * time.Minute
)

// ReserveReporter interface defines the functionality required from the local storage
// of the node to report information about the reserve. The reserve storage is the storage
// pledged by the node to the network.
type ReserveReporter interface {
	// Current size of the reserve.
	Size() (uint64, error)
	// Capacity of the reserve that is configured.
	Capacity() uint64
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
	topology.SetStorageDepther
}

// Service implements the depthmonitor service
type Service struct {
	depthLock        sync.Mutex
	topology         Topology
	syncer           SyncReporter
	reserve          ReserveReporter
	st               storage.StateStorer
	logger           logging.Logger
	quit             chan struct{}
	storageDepth     uint8
	oldStorageRadius uint8
}

// New constructs a new depthmonitor service
func New(
	t Topology,
	syncer SyncReporter,
	reserve ReserveReporter,
	st storage.StateStorer,
	logger logging.Logger,
	warmupTime time.Duration,
) *Service {

	s := &Service{
		topology: t,
		syncer:   syncer,
		reserve:  reserve,
		st:       st,
		logger:   logger,
		quit:     make(chan struct{}),
	}

	go s.manage(warmupTime)

	return s
}

func (s *Service) manage(warmupTime time.Duration) {

	// wait for warmup
	select {
	case <-s.quit:
		return
	case <-time.After(warmupTime):
	}

	// check if we have already saved a depth value before shutting down previously.
	var initialDepth uint8
	err := s.st.Get(depthKey, &initialDepth)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		s.logger.Errorf("depthmonitor: failed reading stored depth: %s", err.Error())
	}

	// if we are starting from scratch, we can use the initial ConnectionDepth as a starting point.
	if initialDepth == 0 {
		initialDepth = s.topology.NeighborhoodDepth()
	}

	s.logger.Infof("depthmonitor: warmup period complete, starting worker with initial depth %d", initialDepth)

	s.depthLock.Lock()
	s.storageDepth = initialDepth
	s.depthLock.Unlock()

	defer s.putStorageDeth()

	halfCapacity := float64(s.reserve.Capacity()) / 2

	isHalfFull := func(s float64) bool {
		return s > halfCapacity
	}

	var (
		adaptationPeriod bool
		adaptationStart  time.Time // start of the adaptation window
		adaptationWindow float64   // allowed time in seconds to fill upto half of the reserve
	)

	for {
		select {
		case <-s.quit:
			return
		case <-time.After(manageWait):
		}

		size, err := s.reserve.Size()
		if err != nil {
			s.logger.Errorf("depthmonitor: reserve size %v", err)
			continue
		}

		currentSize := float64(size)

		// if we have crossed 50% utilization, dont do anything
		if isHalfFull(currentSize) {
			adaptationPeriod = false
			continue
		}

		// if we dont have 50% utilization of the reserve, enter into an adaptation period
		// to see if we need to modify the depth to improve utilization
		// using a rate of window_size / half_reserve, compute max adaption window time allowed to fill the reserve
		if !adaptationPeriod {
			adaptationPeriod = true
			adaptationStart = time.Now()
			rate := adaptationWindowSeconds / halfCapacity
			emptySize := halfCapacity - currentSize
			adaptationWindow = rate * emptySize
			s.logger.Infof("depthmonitor: starting adaptation period with window time %v", time.Second*time.Duration(adaptationWindow))
		}

		// edge case, if we have crossed the adaptation window, roll it back a little to allow sync to fill the reserve
		if time.Since(adaptationStart).Seconds() > adaptationWindow {
			adaptationStart = time.Now().Add(-time.Minute * adaptationRollbackMinutes)
			s.logger.Infof("depthmonitor: rolling back adaptation window to allow sync to fill reserve")
		}

		// based on the sync rate, determine the expected size of reserve at the end of the
		// adaptation window
		timeleft := adaptationWindow - time.Since(adaptationStart).Seconds()
		expectedSize := s.syncer.Rate()*timeleft + currentSize

		s.logger.Infof("depthmonitor: expected size %.0f with current size %.0f and pullsync rate %.2f ch/s, time left %v", expectedSize, currentSize, s.syncer.Rate(), time.Second*time.Duration(timeleft))

		// if we are in the adaptation window and we are not expecting to have enough utilization
		// by the end of it, we proactively decrease the storage depth to allow nodes to widen
		// their neighbourhoods
		if !isHalfFull(expectedSize) {
			s.depthLock.Lock()
			if s.storageDepth > 0 {
				s.storageDepth--
				s.topology.SetStorageDepth(s.storageDepth)
				s.logger.Infof("depthmonitor: reducing storage depth to %d", s.storageDepth)
				s.putStorageDeth()
			}
			s.depthLock.Unlock()
		}
	}
}

func (s *Service) Close() error {
	close(s.quit)
	return nil
}

// SetStorageRadius implements the postage.RadiusSetter interface to receive updates from batchstore
// about the storageRadius based on on-chain events
func (s *Service) SetStorageRadius(storageRadius uint8) {

	s.depthLock.Lock()
	defer s.depthLock.Unlock()

	if (storageRadius > s.storageDepth) || (s.oldStorageRadius == s.storageDepth && storageRadius < s.oldStorageRadius) {
		s.storageDepth = storageRadius
		s.topology.SetStorageDepth(s.storageDepth)
		s.logger.Infof("depthmonitor: setting storage depth to %d", s.storageDepth)
		s.putStorageDeth()
	}

	s.oldStorageRadius = storageRadius
}

func (s *Service) putStorageDeth() {
	err := s.st.Put(depthKey, s.storageDepth)
	if err != nil {
		s.logger.Errorf("depthmonitor: failed updating storage depth: %v", err)
	}
}
