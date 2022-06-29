package depthmonitor

import (
	"errors"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
)

const (
	depthKey         string  = "storage_depth"
	adaptationWindow float64 = 60 * 60
	manageWait               = 5 * time.Minute
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
	// Get the connection depth from topology.
	ConnectionDepth() int
	// Set the radius of responsibility. This can be used by the node to define
	// neighbourhoods.
	SetRadius(int)
}

type Service struct {
	topo         Topology
	syncer       SyncReporter
	reserve      ReserveReporter
	st           storage.StateStorer
	logger       logging.Logger
	depthLock    sync.Mutex
	storageDepth int
	quit         chan struct{}
}

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
	initialDepth := 0
	err := s.st.Get(depthKey, &initialDepth)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		s.logger.Errorf("depthmonitor: failed reading stored depth: %s", err.Error())
	}

	// if we are starting from scratch, we can use the initial ConnectionDepth as a starting point.
	if initialDepth == 0 {
		initialDepth = s.topo.ConnectionDepth()
	}

	s.logger.Infof("depthmonitor: warmup period complete, starting worker with initial depth %d", initialDepth)

	s.depthLock.Lock()
	s.storageDepth = initialDepth
	s.depthLock.Unlock()

	isNotHalfFull := func(x, y float64) bool {
		return x/y < 0.5
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

		// if
		if time.Now().Sub(adaptationStart).Seconds() > adaptationWindow {
			adaptationPeriod = false
		}

		currentSize := float64(s.reserve.Size())
		capacity := float64(s.reserve.Capacity())
		reserveNotHalfFull := isNotHalfFull(currentSize, capacity)

		if reserveNotHalfFull && !adaptationPeriod {
			adaptationPeriod = true
			adaptationStart = time.Now()
		}

		if !reserveNotHalfFull {
			adaptationPeriod = false
		}

		expectedSize := s.syncer.Rate()*(adaptationWindow-time.Now().Sub(adaptationStart).Seconds()) + currentSize

		if adaptationPeriod && isNotHalfFull(expectedSize, capacity) {
			s.depthLock.Lock()
			s.storageDepth--
			s.topo.SetRadius(s.storageDepth)
			s.logger.Infof("depthmonitor: reducing storage depth to better utilize reserve, new depth: %d", s.storageDepth)
			s.depthLock.Unlock()
			adaptationPeriod = false
		}
	}
}

func (s *Service) SetRadius() {}
