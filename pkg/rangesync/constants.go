// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/storer"
)

// Session constants (proposal §6.3).
const (
	// MinBatch is the smallest number of coded symbols in a first batch.
	MinBatch = 4
	// BatchFactor scales the estimated difference into the size of the first
	// batch: ceil(BatchFactor * estimate) + 2 symbols.
	BatchFactor = 1.6
	// GrowthFactor is how much a symbol stream grows on each More request.
	GrowthFactor = 4

	// ItemsPageSize is the largest number of IDs in one Items part.
	ItemsPageSize = 1024

	// LargeDiff is the estimated difference above which the initiator hands a
	// range to the reconciler for a striped transfer instead of reconciling it.
	LargeDiff = 65536
)

// Message and session limits (proposal §6.2, §6.4).
const (
	// LeafDepthSlack is how many bits below the session leaf depth an incoming
	// range may lie, to allow a peer with deeper leaves (the "+4 bits" of
	// §6.4). It is one split level.
	LeafDepthSlack = fingerprint.FanoutBits

	// MaxParts is the largest number of parts in one Message.
	MaxParts = 1024
	// MaxMessageBytes is the largest encoded size of one Message.
	MaxMessageBytes = 1 << 20

	// SessionSymbolBudget is the largest number of coded symbols a responder
	// sends in one session; past it every range is answered as exhausted.
	SessionSymbolBudget = 16384
	// MaxBatchSymbols is the largest number of coded symbols in one Symbols
	// part: 8192 × fingerprint.MaxSymbolSize is 720 KiB, so a batch always
	// fits in a Message.
	MaxBatchSymbols = 8192
	// HintCacheSize is the number of (peer, range) entries of the decoded
	// difference sizes a node keeps for the symbol estimate hint(R).
	HintCacheSize = 4096
	// MaxScansPerMessage is the largest number of ranges in one Message whose
	// symbols are produced by scanning the address index.
	MaxScansPerMessage = 64

	// MaxPendingReplies is the largest number of the peer's Reply pages
	// whose answers wait for room in a later Message and that a session
	// keeps with the listed IDs it must not send back. Pages past it are
	// answered without that exclusion (at most 16 × ItemsPageSize stored
	// IDs: 1 MiB of IDs, about 1.2–1.3 MiB with the map overhead). Answers
	// sent in the Message built right after the page arrived always keep
	// it.
	MaxPendingReplies = 16

	// MaxPeerItems is the most IDs a peer may list in one range of a
	// session: the largest reserve, DefaultReserveCapacity with the one
	// allowed capacity doubling.
	MaxPeerItems = 2 * storer.DefaultReserveCapacity

	// MaxWantIDs is the largest number of IDs in one Want.
	MaxWantIDs = 250

	// MaxConcurrentSessions is the largest number of reconcile sessions a node
	// serves at once; sessions over the cap are answered with Busy.
	MaxConcurrentSessions = 16
	// MaxConcurrentSessionsPerPeer is the largest number of reconcile sessions
	// a node serves for one peer at once.
	MaxConcurrentSessionsPerPeer = 1
	// PeerSessionBurst is the number of sessions a peer may open back to
	// back; after it, one per PeerSessionInterval. Sessions over the rate
	// are answered with Busy (proposal §9, Status spam).
	PeerSessionBurst = 4
)

// Scheduling constants (proposal §6.1, §8.2, §8.4).
const (
	// StatusMinInterval is the debounce interval of Status gossip.
	StatusMinInterval = 5 * time.Second
	// StatusMaxInterval is the Status heartbeat interval.
	StatusMaxInterval = 60 * time.Second

	// MinRoundInterval is the shortest interval between reconciliation rounds,
	// used while rounds keep finding differences.
	MinRoundInterval = 5 * time.Second
	// MaxRoundInterval is the longest interval between reconciliation rounds,
	// used while every digest matches.
	MaxRoundInterval = 30 * time.Second
	// PeerSessionInterval is the interval at which a peer regains one
	// session of PeerSessionBurst: half the shortest round interval, so an
	// honest initiator never hits it.
	PeerSessionInterval = MinRoundInterval / 2

	// ClaimDeadline is how long a claimed ID stays in the want set before it
	// can be claimed through another peer.
	ClaimDeadline = 30 * time.Second
	// RejectTTL is how long a rejected ID is not fetched again.
	RejectTTL = 10 * time.Minute

	// DefaultFetchRate is the default global fetch limit in chunks per second.
	DefaultFetchRate = 1000

	// CatchUpMinDiff is the smallest resolved difference of a session for its
	// fetches to count towards the catch-up sync rate.
	CatchUpMinDiff = 256
	// SyncRateWindow is the window over which the catch-up sync rate is
	// measured; it is the same as the puller's historical rate window.
	SyncRateWindow = 10 * time.Minute
	// StartupGuardTimeout is the longest time after start during which the
	// sync rate reports a non-zero sentinel while the first status exchange
	// with the neighbors is still pending.
	StartupGuardTimeout = 2 * StatusMaxInterval
)
