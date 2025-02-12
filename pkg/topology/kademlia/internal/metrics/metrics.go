// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package metrics provides service for collecting various metrics about peers.
// It is intended to be used with the kademlia where the metrics are collected.
package metrics

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/shed"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/syndtr/goleveldb/leveldb"
)

const ewmaSmoothing = 0.1

// PeerConnectionDirection represents peer connection direction.
type PeerConnectionDirection string

const (
	PeerConnectionDirectionInbound  PeerConnectionDirection = "inbound"
	PeerConnectionDirectionOutbound PeerConnectionDirection = "outbound"
)

// RecordOp is a definition of a peer metrics Record
// operation whose execution modifies a specific metrics.
type RecordOp func(*Counters)

// Bootnode will mark the peer metric as bootnode based on the bool arg.
func IsBootnode(b bool) RecordOp {
	return func(cs *Counters) {
		cs.Lock()
		defer cs.Unlock()
		cs.IsBootnode = b
	}
}

// PeerLogIn will first update the current last seen to the give time t and as
// the second it'll set the direction of the session connection to the given
// value. The force flag will force the peer re-login if he's already logged in.
// The time is set as Unix timestamp ignoring the timezone. The operation will
// panic if the given time is before the Unix epoch.
func PeerLogIn(t time.Time, dir PeerConnectionDirection) RecordOp {
	return func(cs *Counters) {
		cs.Lock()
		defer cs.Unlock()

		if cs.isLoggedIn {
			return // Ignore when the peer is already logged in.
		}
		cs.isLoggedIn = true

		ls := t.UnixNano()
		if ls < 0 {
			panic(fmt.Errorf("time before unix epoch: %s", t))
		}
		cs.sessionConnDirection = dir
		cs.lastSeenTimestamp = ls
	}
}

// PeerLogOut will first update the connection session and total duration with
// the difference of the given time t and the current last seen value. As the
// second it'll also update the last seen peer metrics to the given time t.
// The time is set as Unix timestamp ignoring the timezone. The operation will
// panic if the given time is before the Unix epoch.
func PeerLogOut(t time.Time) RecordOp {
	return func(cs *Counters) {
		cs.Lock()
		defer cs.Unlock()

		if !cs.isLoggedIn {
			return // Ignore when the peer is not logged in.
		}
		cs.isLoggedIn = false

		curLs := cs.lastSeenTimestamp
		newLs := t.UnixNano()
		if newLs < 0 {
			panic(fmt.Errorf("time before unix epoch: %s", t))
		}

		cs.sessionConnDuration = time.Duration(newLs - curLs)
		cs.connTotalDuration += cs.sessionConnDuration
		cs.lastSeenTimestamp = newLs
	}
}

// IncSessionConnectionRetry increments the session connection retry
// counter by 1.
func IncSessionConnectionRetry() RecordOp {
	return func(cs *Counters) {
		cs.Lock()
		defer cs.Unlock()

		cs.sessionConnRetry++
	}
}

// PeerLatency records the average peer latency.
func PeerLatency(t time.Duration) RecordOp {
	return func(cs *Counters) {
		cs.Lock()
		defer cs.Unlock()
		// short circuit the first measurement
		if cs.latencyEWMA == 0 {
			cs.latencyEWMA = t
			return
		}
		v := (ewmaSmoothing * float64(t)) + (1-ewmaSmoothing)*float64(cs.latencyEWMA)
		cs.latencyEWMA = time.Duration(v)
	}
}

// PeerReachability updates the last reachability status.
func PeerReachability(s p2p.ReachabilityStatus) RecordOp {
	return func(cs *Counters) {
		cs.Lock()
		defer cs.Unlock()
		cs.ReachabilityStatus = s
	}
}

// PeerHealth updates the last health status of a peers.
func PeerHealth(isHealty bool) RecordOp {
	return func(cs *Counters) {
		cs.Lock()
		defer cs.Unlock()
		cs.Healthy = isHealty
	}
}

// Snapshot represents a snapshot of peers' metrics counters.
type Snapshot struct {
	LastSeenTimestamp          int64
	SessionConnectionRetry     uint64
	ConnectionTotalDuration    time.Duration
	SessionConnectionDuration  time.Duration
	SessionConnectionDirection PeerConnectionDirection
	LatencyEWMA                time.Duration
	Reachability               p2p.ReachabilityStatus
	Healthy                    bool
	IsBootnode                 bool
}

// persistentCounters is a helper struct used for persisting selected counters.
type persistentCounters struct {
	PeerAddress       swarm.Address `json:"peerAddress"`
	LastSeenTimestamp int64         `json:"lastSeenTimestamp"`
	ConnTotalDuration time.Duration `json:"connTotalDuration"`
	IsBootnode        bool          `json:"isBootnode"`
}

// Counters represents a collection of peer metrics
// mainly collected for statistics and debugging.
type Counters struct {
	sync.Mutex

	// Bookkeeping.
	isLoggedIn  bool
	peerAddress swarm.Address
	IsBootnode  bool

	// Counters.
	lastSeenTimestamp    int64
	connTotalDuration    time.Duration
	sessionConnRetry     uint64
	sessionConnDuration  time.Duration
	sessionConnDirection PeerConnectionDirection
	latencyEWMA          time.Duration
	ReachabilityStatus   p2p.ReachabilityStatus
	Healthy              bool
}

// UnmarshalJSON unmarshal just the persistent counters.
func (cs *Counters) UnmarshalJSON(b []byte) (err error) {
	var val persistentCounters
	if err := json.Unmarshal(b, &val); err != nil {
		return err
	}
	cs.Lock()
	cs.peerAddress = val.PeerAddress
	cs.lastSeenTimestamp = val.LastSeenTimestamp
	cs.connTotalDuration = val.ConnTotalDuration
	cs.IsBootnode = val.IsBootnode
	cs.Unlock()
	return nil
}

// MarshalJSON marshals just the persistent counters.
func (cs *Counters) MarshalJSON() ([]byte, error) {
	cs.Lock()
	val := persistentCounters{
		PeerAddress:       cs.peerAddress,
		LastSeenTimestamp: cs.lastSeenTimestamp,
		ConnTotalDuration: cs.connTotalDuration,
		IsBootnode:        cs.IsBootnode,
	}
	cs.Unlock()
	return json.Marshal(val)
}

// snapshot returns current snapshot of counters referenced to the given t.
func (cs *Counters) snapshot(t time.Time) *Snapshot {
	cs.Lock()
	defer cs.Unlock()

	connTotalDuration := cs.connTotalDuration
	sessionConnDuration := cs.sessionConnDuration
	if cs.isLoggedIn {
		sessionConnDuration = t.Sub(time.Unix(0, cs.lastSeenTimestamp))
		connTotalDuration += sessionConnDuration
	}

	return &Snapshot{
		LastSeenTimestamp:          cs.lastSeenTimestamp,
		SessionConnectionRetry:     cs.sessionConnRetry,
		ConnectionTotalDuration:    connTotalDuration,
		SessionConnectionDuration:  sessionConnDuration,
		SessionConnectionDirection: cs.sessionConnDirection,
		LatencyEWMA:                cs.latencyEWMA,
		Reachability:               cs.ReachabilityStatus,
		Healthy:                    cs.Healthy,
		IsBootnode:                 cs.IsBootnode,
	}
}

// NewCollector is a convenient constructor for creating new Collector.
func NewCollector(db *shed.DB) (*Collector, error) {
	const name = "kademlia-counters"

	c := new(Collector)

	val, err := db.NewStructField(name)
	if err != nil {
		return nil, fmt.Errorf("field initialization for %q failed: %w", name, err)
	}
	c.persistence = &val

	counters := make(map[string]persistentCounters)
	if err := val.Get(&counters); err != nil && !errors.Is(err, leveldb.ErrNotFound) {
		return nil, err
	}

	for _, val := range counters {
		c.counters.Store(val.PeerAddress.ByteString(), &Counters{
			peerAddress:       val.PeerAddress,
			lastSeenTimestamp: val.LastSeenTimestamp,
			connTotalDuration: val.ConnTotalDuration,
			IsBootnode:        val.IsBootnode,
		})
	}

	return c, nil
}

// Collector collects various metrics about
// peers specified be the swarm.Address.
type Collector struct {
	counters    sync.Map
	persistence *shed.StructField
}

// Record records a set of metrics for peer specified by the given address.
func (c *Collector) Record(addr swarm.Address, rop ...RecordOp) {
	val, _ := c.counters.LoadOrStore(addr.ByteString(), &Counters{peerAddress: addr})
	for _, op := range rop {
		op(val.(*Counters))
	}
}

// Snapshot returns the current state of the metrics collector for peer(s).
// The given time t is used to calculate the duration of the current session,
// if any. If an address or a set of addresses is specified then only metrics
// related to them will be returned, otherwise metrics for all peers will be
// returned. If the peer is still logged in, the session-related counters will
// be evaluated against the last seen time, which equals to the login time. If
// the peer is logged out, then the session counters will reflect its last
// session.
func (c *Collector) Snapshot(t time.Time, addresses ...swarm.Address) map[string]*Snapshot {
	snapshot := make(map[string]*Snapshot)

	for _, addr := range addresses {
		val, ok := c.counters.Load(addr.ByteString())
		if !ok {
			continue
		}
		cs := val.(*Counters)
		snapshot[addr.ByteString()] = cs.snapshot(t)
	}

	if len(addresses) == 0 {
		c.counters.Range(func(key, val interface{}) bool {
			cs := val.(*Counters)
			snapshot[cs.peerAddress.ByteString()] = cs.snapshot(t)
			return true
		})
	}

	return snapshot
}

// IsUnreachable returns true if the peer is unreachable.
func (c *Collector) IsUnreachable(addr swarm.Address) bool {
	val, ok := c.counters.Load(addr.ByteString())
	if !ok {
		return true
	}
	cs := val.(*Counters)

	cs.Lock()
	defer cs.Unlock()

	return cs.ReachabilityStatus != p2p.ReachabilityStatusPublic
}

// ExcludeOp is a function type used to filter peers on certain fields.
type ExcludeOp func(*Counters) bool

// IsBootnode is used to filter bootnode peers.
func Bootnode() ExcludeOp {
	return func(cs *Counters) bool {
		return cs.IsBootnode
	}
}

// Reachable is used to filter reachable or unreachable peers based on r.
func Reachability(filterReachable bool) ExcludeOp {
	return func(cs *Counters) bool {
		reachble := cs.ReachabilityStatus == p2p.ReachabilityStatusPublic
		if filterReachable {
			return reachble
		}
		return !reachble
	}
}

// Unreachable is used to filter unhealthy peers.
func Health(filterHealthy bool) ExcludeOp {
	return func(cs *Counters) bool {
		if filterHealthy {
			return cs.Healthy
		}
		return !cs.Healthy
	}
}

// Exclude returns false if the addr passes all exclusion operations.
func (c *Collector) Exclude(addr swarm.Address, fop ...ExcludeOp) bool {
	val, ok := c.counters.Load(addr.ByteString())
	if !ok {
		return true
	}
	cs := val.(*Counters)
	cs.Lock()
	defer cs.Unlock()

	for _, f := range fop {
		if f(cs) {
			return true
		}
	}

	return false
}

// Inspect allows inspecting current snapshot for the given
// peer address by executing the inspection function.
func (c *Collector) Inspect(addr swarm.Address) *Snapshot {
	snapshots := c.Snapshot(time.Now(), addr)
	return snapshots[addr.ByteString()]
}

// Flush sync the dirty in memory counters for all peers by flushing their
// values to the underlying storage.
func (c *Collector) Flush() error {
	counters := make(map[string]interface{})
	c.counters.Range(func(key, val interface{}) bool {
		cs := val.(*Counters)
		counters[cs.peerAddress.ByteString()] = val
		return true
	})

	if err := c.persistence.Put(counters); err != nil {
		return fmt.Errorf("unable to persist counters: %w", err)
	}
	return nil
}

// Finalize tries to log out all ongoing peer sessions.
func (c *Collector) Finalize(t time.Time, remove bool) error {
	c.counters.Range(func(_, val interface{}) bool {
		cs := val.(*Counters)
		PeerLogOut(t)(cs)
		return true
	})

	if err := c.Flush(); err != nil {
		return err
	}

	if remove {
		c.counters.Range(func(_, val interface{}) bool {
			cs := val.(*Counters)
			c.counters.Delete(cs.peerAddress.ByteString())
			return true
		})
	}

	return nil
}
