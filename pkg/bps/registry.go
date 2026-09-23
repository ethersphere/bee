package bps

import (
	"sync"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func NewRegistry() *CohortRegistry {
	return &CohortRegistry{
		cohorts: make(map[string]*Cohort),
	}
}

type CohortRegistry struct {
	mtx     sync.Mutex
	cohorts map[string]*Cohort
}

// Jopen is a join/open operation in one - the peer either joins an existing
// cohort or creates one by joining a previously unknown topic.
func (c *CohortRegistry) Jopen(overlay swarm.Address, topic []byte) *Cohort {
	return nil
}

type Cohort struct {
	mtx       sync.Mutex
	topic     []byte // topic is the feed topic + owner hash
	lastSeen  uint64 // last feed update index, used to prevent replay and circumvent dedup logic (for now)
	members   map[string]string
	publisher swarm.Address
	challenge []byte
}
