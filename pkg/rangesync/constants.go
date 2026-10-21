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
	// batch: ceil(BatchFactor * estimate) + 2 symbols. It is
	// BatchFactorNum / BatchFactorDen, the fraction FirstBatch computes with
	// in integers; change those to change it.
	BatchFactor = 1.0 * BatchFactorNum / BatchFactorDen
	// BatchFactorNum and BatchFactorDen are BatchFactor as a fraction, both at
	// least 1.
	BatchFactorNum = 16
	BatchFactorDen = 10
	// GrowthFactor is how much a symbol stream grows on each More request.
	GrowthFactor = 4

	// ItemsPageSize is the largest number of IDs in one Items part.
	ItemsPageSize = 1024

	// LargeDiff is the count difference above which the side of a session
	// that holds fewer elements in a range, initiator or responder, hands
	// the range to its reconciler for a striped transfer instead of
	// reconciling it (DecisionStripe). Options.LargeDiff scales it in tests.
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
	// PeerStatusBurst is the number of Statuses a peer may send back to
	// back; after it, one per PeerStatusInterval. Statuses over the rate are
	// dropped unread (proposal §9, Status spam).
	PeerStatusBurst = 4
	// PeerStatusDropBurst is the number of Statuses over its rate a peer may
	// send (each dropped) back to back; after it, one per
	// PeerStatusDropInterval. A Status over the rate past them is a
	// violation: the peer keeps sending faster than PeerStatusBurst and
	// PeerStatusInterval allow (proposal §9, Status spam). An honest sender
	// stays a Status below the receiver's rate (the reconciler's sender-side
	// cap), so only delayed arrivals drop any of its Statuses, far fewer.
	PeerStatusDropBurst = 2 * PeerStatusBurst
)

// Session memory budget of the symbol path (proposal §6.4, T4.3). A session
// receives at most SessionSymbolBudget coded symbols over all its streams,
// and its decoders recover at most two IDs per received symbol (more is
// fingerprint.ErrCorrupt). A decoder keeps every received symbol and every
// recovered ID until the session ends. A node runs at most
// MaxConcurrentSessions sessions, served and initiated together, so
// NodeDecoderMemory bounds its decoders. Not counted here, and bounded
// elsewhere: the cached symbol prefixes a session copies when a symbol
// stream starts (at most cap(R) symbols per open stream), the listed IDs
// kept for the peer's Reply pages (MaxPendingReplies), the wants a session
// returns (a range this node lacks by more than LargeDiff goes to the
// Striper instead), and one Message in each direction (MaxMessageBytes).
const (
	// DecoderBytesPerID bounds the memory a decoder keeps per recovered ID:
	// its coded symbol and mapping iterator (to remove it from the symbols
	// that arrive later), its entry in the set of recovered IDs and in the
	// result. TestSessionDecoderMemory measures about 110 B on amd64.
	DecoderBytesPerID = 200
	// codedSymbolBytes is the in-memory size of one fingerprint.CodedSymbol:
	// its sum, its checksum and its int64 count.
	codedSymbolBytes = fingerprint.IDSize + fingerprint.ChecksumSize + 8
	// SessionDecoderMemory is the most memory the decoders of one session
	// keep: SessionSymbolBudget received symbols (1.4 MiB) and twice as many
	// recovered IDs (6.25 MiB).
	SessionDecoderMemory = SessionSymbolBudget*codedSymbolBytes + 2*SessionSymbolBudget*DecoderBytesPerID
	// NodeDecoderMemory is the most memory the decoders of all the sessions
	// of a node keep (about 122 MiB).
	NodeDecoderMemory = MaxConcurrentSessions * SessionDecoderMemory
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
	// PeerStatusInterval is the interval at which a peer regains one Status
	// of PeerStatusBurst: half of StatusMinInterval, so an honest peer,
	// which sends a neighbor at most one Status per StatusMinInterval apart
	// from the first contact and a radius change, never hits it.
	PeerStatusInterval = StatusMinInterval / 2
	// PeerStatusDropInterval is the interval at which a peer regains one
	// dropped Status of PeerStatusDropBurst. A peer that sends at most a
	// quarter more Statuses than its rate is not escalated: a dropped
	// Status costs this node a stream reset before anything is read.
	PeerStatusDropInterval = 4 * PeerStatusInterval

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
