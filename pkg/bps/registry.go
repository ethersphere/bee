package bps

import (
	"crypto/rand"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func NewRegistry() *CohortRegistry {
	return &CohortRegistry{
		cohorts: make(map[string]*cohort),
	}
}

type CohortRegistry struct {
	mtx     sync.Mutex
	cohorts map[string]*cohort
}

// join/open operation in one - the peer either joins an existing
// cohort or creates one by joining a previously unregistered topic.
// returns the cohort challenge
func (c *CohortRegistry) Join(overlay swarm.Address, topic []byte, ch chan []byte) []byte {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	t := string(topic)
	co, ok := c.cohorts[t]
	if ok {
		co.members[overlay.String()] = ch
		return co.challenge
	}
	members := make(map[string]chan []byte)
	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		panic(err)
	}

	members[overlay.String()] = ch
	c.cohorts[t] = &cohort{
		topic:     topic,
		members:   members,
		challenge: challenge,
	}

	return challenge
}

type cohort struct {
	// mtx        sync.Mutex
	topic     []byte // topic is the feed topic + owner hash
	lastSeen  uint64 // last feed update index, used to prevent replay and circumvent dedup logic (for now)
	members   map[string]chan []byte
	publisher swarm.Address
	challenge []byte
}
