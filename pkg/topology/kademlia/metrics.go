// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kademlia

import (
	"fmt"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/hashicorp/go-multierror"
)

// peerMetricsKeyPrefix specifies the type for defining peer metrics prefix key
// necessary for identification of the values stored in persistent store.
type peerMetricsKeyPrefix string

// peerConnectionDirection represents peer connection direction.
type peerConnectionDirection string

const (
	peerLastSeen                peerMetricsKeyPrefix = "peer-last-seen"
	peerTotalConnectionDuration peerMetricsKeyPrefix = "peer-total-connection-duration"

	peerConnectionDirectionInbound  peerConnectionDirection = "inbound"
	peerConnectionDirectionOutbound peerConnectionDirection = "outbound"
)

// peerMetricsKey helps with definition of the peer metrics keys.
type peerMetricsKey struct {
	prefix  peerMetricsKeyPrefix
	address string
}

// String implements Stringer.String method.
func (mk peerMetricsKey) String() string {
	return fmt.Sprintf("%s-%s", mk.prefix, mk.address)
}

// newPeerMetricsKey is a convenient constructor for creating new peerMetricsKey.
func newPeerMetricsKey(p peerMetricsKeyPrefix, a string) *peerMetricsKey {
	return &peerMetricsKey{
		prefix:  p,
		address: a,
	}
}

// peerMetricsOp is a definition of a peer metrics operation
// whose execution modifies a specific metrics.
type peerMetricsOp func(*peerMetrics) error

// setLastSeen sets the last seen peer metrics to the given time.
// The time is set as unix timestamp ignoring timezone. The
// operation will panics if the given time is before the unix epoch.
func setLastSeen(time time.Time) peerMetricsOp {
	return func(m *peerMetrics) error {
		t := time.UnixNano()
		if t < 0 {
			panic(fmt.Errorf("time before unix epoch: %s", time))
		}
		return m.lastSeen.Put(uint64(t))
	}
}

// setConnectionTotalDuration sets the connection total duration metrics to the
// given value. The operation will panics if the given duration is of negative
// value.
func setConnectionTotalDuration(d time.Duration) peerMetricsOp {
	return func(m *peerMetrics) error {
		if d < 0 {
			panic(fmt.Errorf("negative duration: %d", d))
		}
		return m.connTotalDuration.Put(uint64(d))
	}
}

// incSessionConnectionRetry increments the
// session connection retry counter by 1.
func incSessionConnectionRetry() peerMetricsOp {
	return func(m *peerMetrics) error {
		m.sessionConnRetry++
		return nil
	}
}

// addSessionConnectionDuration adds the given duration
// to the session connection duration counter.
func addSessionConnectionDuration(d time.Duration) peerMetricsOp {
	return func(m *peerMetrics) error {
		m.sessionConnDuration += d
		return nil
	}
}

// setSessionConnectionDirection sets the session connection direction to the
// given value.
func setSessionConnectionDirection(d peerConnectionDirection) peerMetricsOp {
	return func(m *peerMetrics) error {
		m.sessionConnDirection = d
		return nil
	}
}

// peerMetricsSnapshot represents snapshot of peer's metrics.
type peerMetricsSnapshot struct {
	LastSeen                   time.Time               `json:"lastSeen"`
	ConnectionTotalDuration    time.Duration           `json:"connectionTotalDuration"`
	SessionConnectionRetry     int                     `json:"sessionConnectionRetry"`
	SessionConnectionDuration  time.Duration           `json:"sessionConnectionDuration"`
	SessionConnectionDirection peerConnectionDirection `json:"sessionConnectionDirection"`
}

// peerMetrics represents a collection of peer metrics
// mainly collected for statistics and debugging.
type peerMetrics struct {
	// Persistent.
	lastSeen          *shed.Uint64Field
	connTotalDuration *shed.Uint64Field
	// In memory.
	sessionConnRetry     int
	sessionConnDuration  time.Duration
	sessionConnDirection peerConnectionDirection
}

// newPeerMetricsCollector is a convenient constructor
// for creating new peerMetricsCollector.
func newPeerMetricsCollector(db *shed.DB) *peerMetricsCollector {
	return &peerMetricsCollector{
		db:   db,
		data: make(map[string]*peerMetrics),
	}
}

// peerMetricsCollector collects various metrics about
// connected peers specified be the swarm.Address.
type peerMetricsCollector struct {
	db   *shed.DB
	mu   sync.RWMutex // mu guards data.
	data map[string]*peerMetrics
}

// metrics records a set of metrics for peer specified by the given address.
// The execution doesn't stop if some metric operation returns an error, it
// rather continues and all the execution errors are returned.
func (mc *peerMetricsCollector) metrics(addr swarm.Address, mop ...peerMetricsOp) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	key := addr.String()
	m, ok := mc.data[key]
	if !ok {
		mk := newPeerMetricsKey(peerLastSeen, key)
		ls, err := mc.db.NewUint64Field(mk.String())
		if err != nil {
			return fmt.Errorf("field initialization for %q failed: %w", mk, err)
		}

		mk = newPeerMetricsKey(peerTotalConnectionDuration, key)
		cd, err := mc.db.NewUint64Field(mk.String())
		if err != nil {
			return fmt.Errorf("field initialization for %q failed: %w", mk, err)
		}

		m = &peerMetrics{
			lastSeen:          &ls,
			connTotalDuration: &cd,
		}
	}
	mc.data[key] = m

	var err error
	for i, op := range mop {
		if opErr := op(m); opErr != nil {
			err = multierror.Append(err, fmt.Errorf("operation #%d for %q failed: %w", i, key, opErr))
		}
	}
	return err
}

// snapshot writes the current state of the metrics collector to the given writer.
// If an address or a set of addresses is specified then only metrics related
// to them will be written, otherwise metrics for all peers will be written.
// The execution doesn't stop if some metric collection returns an error, it
// rather continues and all the execution errors are returned together with
// the successful peer metrics snapshots.
func (mc *peerMetricsCollector) snapshot(addresses ...swarm.Address) (map[string]*peerMetricsSnapshot, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	var mErr error
	snapshot := make(map[string]*peerMetricsSnapshot)

	take := func(addr string) {
		pm := mc.data[addr]

		var mErr error
		ls, err := pm.lastSeen.Get()
		if err != nil {
			mErr = multierror.Append(mErr, fmt.Errorf("unable to take last seen snapshot for %q: %w", addr, err))
		}

		cn, err := pm.connTotalDuration.Get()
		if err != nil {
			mErr = multierror.Append(mErr, fmt.Errorf("unable to take connection duration snapshot for %q: %w", addr, err))
		}

		snapshot[addr] = &peerMetricsSnapshot{
			LastSeen:                   time.Unix(0, int64(ls)),
			ConnectionTotalDuration:    time.Duration(cn),
			SessionConnectionRetry:     pm.sessionConnRetry,
			SessionConnectionDuration:  pm.sessionConnDuration,
			SessionConnectionDirection: pm.sessionConnDirection,
		}
	}

	for _, addr := range addresses {
		take(addr.String())
	}
	if len(addresses) == 0 {
		for addr := range mc.data {
			take(addr)
		}
	}

	return snapshot, mErr
}
