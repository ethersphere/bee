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
	adaptationWindowSeconds   float64 = 2 * 60 * 60
	adaptationRollbackMinutes         = 5
	manageWait                        = 5 * time.Minute
)

// ReserveReporter interface defines the functionality required from the local storage
// of the node to report information about the reserve. The reserve storage is the storage
// pledged by the node to the network.
type ReserveReporter interface {
	// Current size of the reserve.
	Size() int64
	// Capacity of the reserve that is configured.
	Capacity() int64
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
	topo             Topology
	syncer           SyncReporter
	reserve          ReserveReporter
	st               storage.StateStorer
	logger           logging.Logger
	depthLock        sync.Mutex
	storageDepth     uint8
	quit             chan struct{}
	wg               sync.WaitGroup
	topology         Topology
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
		topo:    t,
		syncer:  syncer,
		reserve: reserve,
		st:      st,
		logger:  logger,
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

	defer func() {
		s.depthLock.Lock()
		defer s.depthLock.Unlock()

		// update the storage depth to statestore before shutting down
		err := s.st.Put(depthKey, s.storageDepth)
		if err != nil {
			s.logger.Errorf("depthmonitor: failed updating storage depth on shutdown: %s", err.Error())
		}
	}()

	capacity := float64(s.reserve.Capacity())

	isNotHalfFull := func(x float64) bool {
		return x/capacity < 0.5
	}

	// will signal that we are in an adaptation period
	adaptationPeriod := false
	// will store the start of the adaptation window
	var adaptationStart time.Time

	for {
		select {
		case <-s.quit:
			return
		case <-time.After(manageWait):
		}

		currentSize := float64(s.reserve.Size())
		reserveNotHalfFull := isNotHalfFull(currentSize)

		// if we have crossed 50% utilization, dont do anything
		if !reserveNotHalfFull {
			adaptationPeriod = false
			continue
		}

		// if we dont have 50% utilization of the reserve, enter into an adaptation period
		// to see if we need to modify the depth to improve utilization
		if !adaptationPeriod {
			adaptationPeriod = true
			adaptationStart = time.Now()
		}

		// edge case, if we have crossed the adaptation window, roll it back a little to allow sync to fill the reserve
		if time.Since(adaptationStart).Seconds() > adaptationWindowSeconds {
			adaptationStart = time.Now().Add(-time.Minute * adaptationRollbackMinutes)
		}

		// based on the sync rate, determine the expected size of reserve at the end of the
		// adaptation window
		expectedSize := s.syncer.Rate()*(adaptationWindowSeconds-time.Since(adaptationStart).Seconds()) + currentSize

		// if we are in the adaptation window and we are not expecting to have enough utilization
		// by the end of it, we proactively decrease the storage depth to allow nodes to widen
		// their neighbourhoods
		if isNotHalfFull(expectedSize) {
			s.depthLock.Lock()
			if s.storageDepth > 0 {
				s.storageDepth--
				s.topology.SetStorageDepth(s.storageDepth)
				s.logger.Infof("depthmonitor: reducing storage depth to depth %d", s.storageDepth)
			}
			s.depthLock.Unlock()
		}
	}
}

func (s *Service) Close() error {
	close(s.quit)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		s.wg.Wait()
	}()

	select {
	case <-stopped:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("stopping depthmonitor with ongoing goroutines")
	}
}

// SetStorageRadius implements the postage.RadiusSetter interface to receive updates from batchstore
// about the storageRadius based on on-chain events
func (s *Service) SetStorageRadius(storageRadius uint8) {

	s.depthLock.Lock()
	defer s.depthLock.Unlock()

	if (storageRadius > s.storageDepth) || (s.oldStorageRadius == s.storageDepth && storageRadius < s.oldStorageRadius) {
		s.storageDepth = storageRadius
		s.topology.SetStorageDepth(s.storageDepth)
	}

	s.oldStorageRadius = storageRadius
}
