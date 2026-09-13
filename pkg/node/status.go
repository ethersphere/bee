// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"sync/atomic"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/storer"
)

// Status is a numeric node startup/runtime phase.
// The zero value is unset and String reports "unknown".
type Status int32

const (
	StatusStarting Status = iota + 1
	StatusWaitingChainSync
	StatusStartingChequebook
	StatusOpeningLocalstore
	StatusResettingReserve
	StatusLoadingPostageSnapshot
	StatusSyncingPostage
	StatusUpdatingStakeHeight
	StatusWarmingUp
	StatusCountingReserve
	StatusEvictingReserve
	StatusSyncingReserve
	StatusReady
)

// String returns the operator-facing name of the status.
func (s Status) String() string {
	switch s {
	case StatusStarting:
		return "starting"
	case StatusWaitingChainSync:
		return "waiting_chain_sync"
	case StatusStartingChequebook:
		return "starting_chequebook"
	case StatusOpeningLocalstore:
		return "opening_localstore"
	case StatusResettingReserve:
		return "resetting_reserve"
	case StatusLoadingPostageSnapshot:
		return "loading_postage_snapshot"
	case StatusSyncingPostage:
		return "syncing_postage"
	case StatusUpdatingStakeHeight:
		return "updating_stake_height"
	case StatusWarmingUp:
		return "warming_up"
	case StatusCountingReserve:
		return "counting_reserve"
	case StatusEvictingReserve:
		return "evicting_reserve"
	case StatusSyncingReserve:
		return "syncing_reserve"
	case StatusReady:
		return "ready"
	default:
		return "unknown"
	}
}

// StatusProvider is the read side of StatusStore.
// The HTTP API depends on api.BeeStatus (same method set, no node import).
type StatusProvider interface {
	Status() Status
	StatusCode() int32
	StatusString() string
}

// StatusStore holds the current Status in an atomic integer.
type StatusStore struct {
	current atomic.Int32
}

var (
	_ StatusProvider = (*StatusStore)(nil)
	_ api.BeeStatus  = (*StatusStore)(nil)
)

// NewStatusStore returns a store with an unset (unknown) status.
func NewStatusStore() *StatusStore {
	return &StatusStore{}
}

// Set stores the given status.
func (s *StatusStore) Set(st Status) {
	if s == nil {
		return
	}
	s.current.Store(int32(st))
}

// Status returns the current status. A nil store is unknown.
func (s *StatusStore) Status() Status {
	if s == nil {
		return 0
	}
	return Status(s.current.Load())
}

// StatusCode returns the current status as a number.
func (s *StatusStore) StatusCode() int32 {
	return int32(s.Status())
}

// StatusString returns the current status name.
func (s *StatusStore) StatusString() string {
	return s.Status().String()
}

func (b *Bee) setStatus(st Status) {
	if b == nil || b.status == nil {
		return
	}
	b.status.Set(st)
	if b.logger != nil {
		b.logger.Info("node status", "status", st.String())
	}
}

func isReserveBlocking(s Status) bool {
	return s == StatusCountingReserve || s == StatusEvictingReserve
}

// setStatusIfIdle writes st unless the reserve worker is in a blocking phase
// or pullsync has already started.
func (b *Bee) setStatusIfIdle(st Status) {
	if b == nil || b.status == nil {
		return
	}
	cur := b.status.Status()
	if isReserveBlocking(cur) || cur == StatusSyncingReserve {
		return
	}
	b.setStatus(st)
}

// setReadyFromWarmup marks the node ready after warmup, but does not interrupt
// reserve counting, eviction, or historical pullsync.
func (b *Bee) setReadyFromWarmup() {
	if b == nil || b.status == nil {
		return
	}
	cur := b.status.Status()
	if isReserveBlocking(cur) || cur == StatusSyncingReserve {
		return
	}
	b.setStatus(StatusReady)
}

// setReadyAfterSync marks the node ready when pullsync has caught up.
func (b *Bee) setReadyAfterSync() {
	if isReserveBlocking(b.status.Status()) {
		return
	}
	b.setStatus(StatusReady)
}

func (b *Bee) applyReservePhase(phase string, syncRate func() float64, stabilized func() bool) {
	switch phase {
	case storer.ReservePhaseCounting:
		b.setStatus(StatusCountingReserve)
	case storer.ReservePhaseEvicting:
		b.setStatus(StatusEvictingReserve)
	case storer.ReservePhaseSyncing:
		if isReserveBlocking(b.status.Status()) {
			return
		}
		b.setStatus(StatusSyncingReserve)
	case storer.ReservePhaseIdle:
		b.setRuntimeIdle(syncRate, stabilized)
	}
}

func (b *Bee) setRuntimeIdle(syncRate func() float64, stabilized func() bool) {
	if stabilized != nil && !stabilized() {
		b.setStatus(StatusWarmingUp)
		return
	}
	if syncRate != nil && syncRate() > 0 {
		b.setStatus(StatusSyncingReserve)
		return
	}
	b.setStatus(StatusReady)
}
