package depthmonitor

import (
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
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
	topo       Topology
	syncer     SyncReporter
	reserve    ReserveReporter
	st         storage.StateStorer
	warmupTime time.Time
}

func New(
	t Topology,
	syncer SyncReporter,
	reserve ReserveReporter,
	st storage.StateStorer,
	warmupTime time.Duration,
	logger logging.Logger,
) *Service {
	return &Service{
		topo:       t,
		syncer:     syncer,
		reserve:    reserve,
		st:         st,
		warmupTime: time.Now().Add(warmupTime),
	}
}
