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
	"github.com/syndtr/goleveldb/leveldb"
	"go.uber.org/atomic"
)

const (
	peerLastSeenTimestamp       string = "peer-last-seen-timestamp"
	peerTotalConnectionDuration string = "peer-total-connection-duration"
)

// PeerConnectionDirection represents peer connection direction.
type PeerConnectionDirection string

const (
	PeerConnectionDirectionInbound  PeerConnectionDirection = "inbound"
	PeerConnectionDirectionOutbound PeerConnectionDirection = "outbound"
)

// peerKey is used to store peers' persistent metrics counters.
type peerKey struct {
	prefix  string
	address string
}

// String implements Stringer.String method.
func (pk peerKey) String() string {
	return fmt.Sprintf("%s-%s", pk.prefix, pk.address)
}

// newPeerKey is a convenient constructor for creating new peerKey.
func newPeerKey(p, a string) *peerKey {
	return &peerKey{
		prefix:  p,
		address: a,
	}
}

// RecordOp is a definition of a peer metrics Record
// operation whose execution modifies a specific metrics.
type RecordOp func(*Counters)

// PeerLogIn will first update the current last seen to the give time t and as
// the second it'll set the direction of the session connection to the given
// value. The force flag will force the peer re-login if he's already logged in.
// The time is set as Unix timestamp ignoring the timezone. The operation will
// panics if the given time is before the Unix epoch.
func PeerLogIn(t time.Time, dir PeerConnectionDirection) RecordOp {
	return func(cs *Counters) {
		cs.Lock()
		defer cs.Unlock()

		if cs.loggedIn {
			return // Ignore when the peer is already logged in.
		}
		cs.loggedIn = true

		ls := t.UnixNano()
		if ls < 0 {
			panic(fmt.Errorf("time before unix epoch: %s", t))
		}
		cs.sessionConnDirection = dir
		cs.lastSeenTimestamp = ls
		cs.dirty.Store(3)
	}
}

// PeerLogOut will first update the connection session and total duration with
// the difference of the given time t and the current last seen value. As the
// second it'll also update the last seen peer metrics to the given time t.
// The time is set as Unix timestamp ignoring the timezone. The operation will
// panics if the given time is before the Unix epoch.
func PeerLogOut(t time.Time) RecordOp {
	return func(cs *Counters) {
		cs.Lock()
		defer cs.Unlock()

		if !cs.loggedIn {
			return // Ignore when the peer is not logged in.
		}
		cs.loggedIn = false

		curLs := cs.lastSeenTimestamp
		newLs := t.UnixNano()
		if newLs < 0 {
			panic(fmt.Errorf("time before unix epoch: %s", t))
		}

		cs.sessionConnDuration = time.Duration(newLs - curLs)
		cs.connTotalDuration += cs.sessionConnDuration
		cs.lastSeenTimestamp = newLs
		cs.dirty.Store(3)
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

// Snapshot represents a snapshot of peers' metrics counters.
type Snapshot struct {
	LastSeenTimestamp          int64
	SessionConnectionRetry     uint64
	ConnectionTotalDuration    time.Duration
	SessionConnectionDuration  time.Duration
	SessionConnectionDirection PeerConnectionDirection
}

// HasAtMaxOneConnectionAttempt returns true if the snapshot represents a new
// peer which has at maximum one session connection attempt but it still isn't
// logged in.
func (ss *Snapshot) HasAtMaxOneConnectionAttempt() bool {
	return ss.LastSeenTimestamp == 0 && ss.SessionConnectionRetry <= 1
}

// Counters represents a collection of peer metrics
// mainly collected for statistics and debugging.
type Counters struct {
	sync.Mutex

	// Bookkeeping.
	peer     *swarm.Address
	loggedIn bool

	// Watches in-memory counters which has to be persisted.
	// 	3 - dirty, need to be persisted
	//	2 - snapshot of counters in progress
	//	1 - batched for persistent write
	//	0 - persisted
	dirty atomic.Int32

	// In-memory counters.
	lastSeenTimestamp    int64
	connTotalDuration    time.Duration
	sessionConnRetry     uint64
	sessionConnDuration  time.Duration
	sessionConnDirection PeerConnectionDirection

	// Persistent counters.
	persistentLastSeenTimestamp atomic.Value
	persistentConnTotalDuration atomic.Value
}

// flush writes the current state of in memory counters into the given db.
func (cs *Counters) flush(db *shed.DB, batch *leveldb.Batch) error {
	if cs.dirty.Load() > 1 {
		return nil
	}
	cs.dirty.CAS(3, 2)

	cs.Lock()
	var (
		key                             = cs.peer.String()
		lastSeenTimestampSnapshot       = cs.lastSeenTimestamp
		connectionTotalDurationSnapshot = cs.connTotalDuration
	)
	cs.Unlock()

	ls, ok := cs.persistentLastSeenTimestamp.Load().(*shed.Uint64Field)
	if !ok {
		mk := newPeerKey(peerLastSeenTimestamp, key)
		field, err := db.NewUint64Field(mk.String())
		if err != nil {
			return fmt.Errorf("field initialization for %q failed: %w", mk, err)
		}
		ls = &field
		cs.persistentLastSeenTimestamp.Store(ls)
	}

	cd, ok := cs.persistentConnTotalDuration.Load().(*shed.Uint64Field)
	if !ok {
		mk := newPeerKey(peerTotalConnectionDuration, key)
		field, err := db.NewUint64Field(mk.String())
		if err != nil {
			return fmt.Errorf("field initialization for %q failed: %w", mk, err)
		}
		cd = &field
		cs.persistentConnTotalDuration.Store(cd)
	}

	ls.PutInBatch(batch, uint64(lastSeenTimestampSnapshot))
	cd.PutInBatch(batch, uint64(connectionTotalDurationSnapshot))

	cs.dirty.CAS(2, 1)

	return nil
}

// snapshot returns current snapshot of counters referenced to the given t.
func (cs *Counters) snapshot(t time.Time) *Snapshot {
	cs.Lock()
	defer cs.Unlock()

	connTotalDuration := cs.connTotalDuration
	sessionConnDuration := cs.sessionConnDuration
	if cs.loggedIn {
		sessionConnDuration = t.Sub(time.Unix(0, cs.lastSeenTimestamp))
		connTotalDuration += sessionConnDuration
	}

	return &Snapshot{
		LastSeenTimestamp:          cs.lastSeenTimestamp,
		SessionConnectionRetry:     cs.sessionConnRetry,
		ConnectionTotalDuration:    connTotalDuration,
		SessionConnectionDuration:  sessionConnDuration,
		SessionConnectionDirection: cs.sessionConnDirection,
	}
}

// NewCollector is a convenient constructor for creating new Collector.
func NewCollector(db *shed.DB) *Collector {
	return &Collector{db: db}
}

// Collector collects various metrics about
// peers specified be the swarm.Address.
type Collector struct {
	db       *shed.DB
	counters sync.Map
}

// Record records a set of metrics for peer specified by the given address.
func (c *Collector) Record(addr swarm.Address, rop ...RecordOp) {
	val, _ := c.counters.LoadOrStore(addr.ByteString(), &Counters{peer: &addr})
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
			snapshot[cs.peer.ByteString()] = cs.snapshot(t)
			return true
		})
	}

	return snapshot
}

// Inspect allows to inspect current snapshot for the given
// peer address by executing the inspection function.
func (c *Collector) Inspect(addr swarm.Address) *Snapshot {
	snapshots := c.Snapshot(time.Now(), addr)
	return snapshots[addr.ByteString()]
}

// Flush sync the dirty in memory counters by flushing their values to the
// underlying storage. If an address or a set of addresses is specified then
// only counters related to them will be flushed, otherwise counters for all
// peers will be flushed.
func (c *Collector) Flush(addresses ...swarm.Address) error {
	var (
		mErr  error
		dirty []string
		batch = new(leveldb.Batch)
	)

	for _, addr := range addresses {
		val, ok := c.counters.Load(addr.ByteString())
		if !ok {
			continue
		}
		cs := val.(*Counters)
		if err := cs.flush(c.db, batch); err != nil {
			mErr = multierror.Append(mErr, fmt.Errorf("unable to batch the counters of peer %q for flash: %w", addr, err))
			continue
		}
		dirty = append(dirty, addr.ByteString())
	}

	if len(addresses) == 0 {
		c.counters.Range(func(_, val interface{}) bool {
			cs := val.(*Counters)
			if err := cs.flush(c.db, batch); err != nil {
				mErr = multierror.Append(mErr, fmt.Errorf("unable to batch the counters of peer %q for flash: %w", cs.peer, err))
				return true
			}
			dirty = append(dirty, cs.peer.ByteString())
			return true
		})
	}

	if batch.Len() == 0 {
		return mErr
	}
	if err := c.db.WriteBatch(batch); err != nil {
		mErr = multierror.Append(mErr, fmt.Errorf("unable to persist counters in batch: %w", err))
	}

	for _, addr := range dirty {
		val, ok := c.counters.Load(addr)
		if !ok {
			continue
		}
		cs := val.(*Counters)
		cs.dirty.CAS(1, 0)
	}

	return mErr
}

// Finalize logs out all ongoing peer sessions
// and flushes all in-memory metrics counters.
func (c *Collector) Finalize(t time.Time) error {
	var (
		mErr  error
		batch = new(leveldb.Batch)
	)

	c.counters.Range(func(_, val interface{}) bool {
		cs := val.(*Counters)
		PeerLogOut(t)(cs)
		if err := cs.flush(c.db, batch); err != nil {
			mErr = multierror.Append(mErr, fmt.Errorf("unable to flush counters for peer %q: %w", cs.peer, err))
		}
		return true
	})

	if batch.Len() > 0 {
		if err := c.db.WriteBatch(batch); err != nil {
			mErr = multierror.Append(mErr, fmt.Errorf("unable to persist counters in batch: %w", err))
		}
	}

	c.counters.Range(func(_, val interface{}) bool {
		cs := val.(*Counters)
		c.counters.Delete(cs.peer.ByteString())
		return true
	})

	return mErr
}
