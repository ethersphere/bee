// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package metrics provides service for collecting various metrics about peers.
// It is intended to be used with the kademlia where the metrics are collected.
package metrics

import (
	"fmt"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/hashicorp/go-multierror"
)

// peerKeyPrefix represents the type for defining peer metrics prefix key
// necessary for identification of the values stored in persistent store.
type peerKeyPrefix string

const (
	peerLastSeen                peerKeyPrefix = "peer-last-seen"
	peerTotalConnectionDuration peerKeyPrefix = "peer-total-connection-duration"
)

// PeerConnectionDirection represents peer connection direction.
type PeerConnectionDirection string

const (
	PeerConnectionDirectionInbound  PeerConnectionDirection = "inbound"
	PeerConnectionDirectionOutbound PeerConnectionDirection = "outbound"
)

// peerKey is used to store peers' persistent metrics counters.
type peerKey struct {
	prefix  peerKeyPrefix
	address string
}

// String implements Stringer.String method.
func (pk peerKey) String() string {
	return fmt.Sprintf("%s-%s", pk.prefix, pk.address)
}

// newPeerKey is a convenient constructor for creating new peerKey.
func newPeerKey(p peerKeyPrefix, a string) *peerKey {
	return &peerKey{
		prefix:  p,
		address: a,
	}
}

// RecordOp is a definition of a peer metrics Record
// operation whose execution modifies a specific metrics.
type RecordOp func(*PeerCounter) error

// PeerLogIn will first update the current last seen to the give time t and as
// the second it'll set the direction of the session connection to the given
// value. The force flag will force the peer re-login if he's already logged in.
// The time is set as Unix timestamp ignoring the timezone. The operation will
// panics if the given time is before the Unix epoch.
func PeerLogIn(t time.Time, dir PeerConnectionDirection) RecordOp {
	return func(pc *PeerCounter) error {
		if pc.loggedIn {
			return nil // Ignore when the peer is already logged in.
		}
		pc.loggedIn = true

		pc.resetSession()

		ls := t.UnixNano()
		if ls < 0 {
			panic(fmt.Errorf("time before unix epoch: %s", t))
		}
		pc.sessionConnDirection = dir
		return pc.lastSeen.Put(uint64(ls))
	}
}

// PeerLogOut will first update the connection session and total duration with
// the difference of the given time t and the current last seen value. As the
// second it'll also update the last seen peer metrics to the given time t.
// The time is set as Unix timestamp ignoring the timezone. The operation will
// panics if the given time is before the Unix epoch.
func PeerLogOut(t time.Time) RecordOp {
	return func(pc *PeerCounter) error {
		if !pc.loggedIn {
			return nil // Ignore when the peer is not logged in.
		}
		pc.loggedIn = false

		newLs := uint64(t.UnixNano())
		if newLs < 0 {
			panic(fmt.Errorf("time before unix epoch: %s", t))
		}

		curLs, err := pc.lastSeen.Get()
		if err != nil {
			return err
		}

		ctd, err := pc.connTotalDuration.Get()
		if err != nil {
			return err
		}

		diff := newLs - curLs
		pc.sessionConnDuration = time.Duration(diff)
		err = pc.connTotalDuration.Put(ctd + diff)
		if err != nil {
			return err
		}
		return pc.lastSeen.Put(newLs)

	}
}

// IncSessionConnectionRetry increments the session connection retry
// counter by 1. The operation will be ignored if peer is not logged in.
func IncSessionConnectionRetry() RecordOp {
	return func(pc *PeerCounter) error {
		if pc.loggedIn {
			pc.sessionConnRetry++
		}
		return nil
	}
}

// Snapshot represents snapshot of peers' metrics in time.
type Snapshot struct {
	LastSeen                   time.Time               `json:"lastSeen"`
	ConnectionTotalDuration    time.Duration           `json:"connectionTotalDuration"`
	SessionConnectionRetry     int                     `json:"sessionConnectionRetry"`
	SessionConnectionDuration  time.Duration           `json:"sessionConnectionDuration"`
	SessionConnectionDirection PeerConnectionDirection `json:"sessionConnectionDirection"`
}

// PeerCounter represents a collection of peer metrics
// mainly collected for statistics and debugging.
type PeerCounter struct {
	loggedIn bool
	// Persistent.
	lastSeen          *shed.Uint64Field
	connTotalDuration *shed.Uint64Field
	// In memory.
	sessionConnRetry     int
	sessionConnDuration  time.Duration
	sessionConnDirection PeerConnectionDirection
}

// resetSession resets the session counters.
func (pc *PeerCounter) resetSession() {
	pc.sessionConnRetry = 0
	pc.sessionConnDirection = ""
}

// NewCollector is a convenient constructor for creating new Collector.
func NewCollector(db *shed.DB) *Collector {
	return &Collector{
		db:       db,
		counters: make(map[string]*PeerCounter),
	}
}

// Collector collects various metrics about
// peers specified be the swarm.Address.
type Collector struct {
	db       *shed.DB
	mu       sync.RWMutex // mu guards counters.
	counters map[string]*PeerCounter
}

// Record records a set of metrics for peer specified by the given address.
// The execution doesn't stop if some metric operation returns an error, it
// rather continues and all the execution errors are returned.
func (c *Collector) Record(addr swarm.Address, rop ...RecordOp) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := addr.String()
	pc, ok := c.counters[key]
	if !ok {
		mk := newPeerKey(peerLastSeen, key)
		ls, err := c.db.NewUint64Field(mk.String())
		if err != nil {
			return fmt.Errorf("field initialization for %q failed: %w", mk, err)
		}

		mk = newPeerKey(peerTotalConnectionDuration, key)
		cd, err := c.db.NewUint64Field(mk.String())
		if err != nil {
			return fmt.Errorf("field initialization for %q failed: %w", mk, err)
		}

		pc = &PeerCounter{
			lastSeen:          &ls,
			connTotalDuration: &cd,
		}
	}
	c.counters[key] = pc

	var err error
	for i, op := range rop {
		if opErr := op(pc); opErr != nil {
			err = multierror.Append(err, fmt.Errorf("operation #%d for %q failed: %w", i, key, opErr))
		}
	}
	return err
}

// Snapshot returns the current state of the metrics collector for peer(s).
// The given time t is used to calculate the duration of the current session,
// if any. If an address or a set of addresses is specified then only metrics
// related to them will be returned, otherwise metrics for all peers will be
// returned. If the peer is still logged in, the session-related counters will
// be evaluated against the last seen time, which equals to the login time. If
// the peer is logged out, then the session counters will reflect its last
// session. The execution doesn't stop if some metric collection returns an
// error, it rather continues and all the execution errors are returned together
// with the successful metrics snapshots.
func (c *Collector) Snapshot(t time.Time, addresses ...swarm.Address) (map[string]*Snapshot, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var mErr error
	snapshot := make(map[string]*Snapshot)

	take := func(addr string) {
		pc := c.counters[addr]
		if pc == nil {
			return
		}

		ls, err := pc.lastSeen.Get()
		if err != nil {
			mErr = multierror.Append(mErr, fmt.Errorf("unable to take last seen snapshot for %q: %w", addr, err))
		}
		lastSeen := time.Unix(0, int64(ls))

		cn, err := pc.connTotalDuration.Get()
		if err != nil {
			mErr = multierror.Append(mErr, fmt.Errorf("unable to take connection duration snapshot for %q: %w", addr, err))
		}

		sessionConnDuration := pc.sessionConnDuration
		if pc.loggedIn {
			sessionConnDuration = t.Sub(lastSeen)
		}

		snapshot[addr] = &Snapshot{
			LastSeen:                   lastSeen,
			ConnectionTotalDuration:    time.Duration(cn),
			SessionConnectionRetry:     pc.sessionConnRetry,
			SessionConnectionDuration:  sessionConnDuration,
			SessionConnectionDirection: pc.sessionConnDirection,
		}
	}

	for _, addr := range addresses {
		take(addr.String())
	}
	if len(addresses) == 0 {
		for addr := range c.counters {
			take(addr)
		}
	}

	return snapshot, mErr
}

// Finalize logs out all ongoing peer sessions
// and flushes all in-memory metrics counters.
func (c *Collector) Finalize(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var mErr error
	for addr, pc := range c.counters {
		if err := PeerLogOut(t)(pc); err != nil {
			mErr = multierror.Append(mErr, fmt.Errorf("unable to logout peer %q: %w", addr, err))
		}
		delete(c.counters, addr)
	}
	return mErr
}
