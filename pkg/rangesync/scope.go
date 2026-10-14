// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"errors"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
)

var (
	// ErrDisjointRoots is returned by NewScope when neither root contains the
	// other: the peers' neighborhoods do not overlap and they do not sync
	// (proposal §7.6).
	ErrDisjointRoots = errors.New("rangesync: disjoint roots")
	// ErrInvalidScope is returned by NewScope when the session root is deeper
	// than the session leaf depth, or when the leaf depth plus LeafDepthSlack
	// exceeds fingerprint.MaxBits. Ranges of such a session could not be split
	// or could not be represented.
	ErrInvalidScope = errors.New("rangesync: invalid session scope")
)

// SessionRoot returns the root of a session between two nodes whose roots are
// a and b (proposal §7.6): the narrower of the two when one contains the
// other, with ok true. When a contains b, the session covers only the part of
// a's neighborhood that b is responsible for; when b contains a (an edge peer
// with a smaller radius), it covers a's neighborhood. Equal roots give that
// root. Disjoint roots give ok false: the nodes do not sync. The result is
// canonical.
func SessionRoot(a, b fingerprint.Range) (root fingerprint.Range, ok bool) {
	return fingerprint.Range{}, false
}

// Scope is the part of the address space that one session reconciles and the
// depth at which it stops splitting ranges. Both peers derive the same Scope
// from the two Status messages. Use NewScope; the zero value is not valid.
type Scope struct {
	// Root is the session root: SessionRoot of the two nodes' roots.
	Root fingerprint.Range
	// LeafDepth is the session leaf depth: the smaller Root.Bits + LeafBits of
	// the two Status messages (proposal §6.3). A range at least this deep is
	// reconciled by exchanging item lists instead of being split.
	LeafDepth uint8
}

// NewScope returns the scope of a session between a node with status local
// and a peer with status remote. It returns an error wrapping
// ErrDisjointRoots when the roots do not overlap and ErrInvalidScope when the
// session root is deeper than the session leaf depth or the leaf depth plus
// LeafDepthSlack exceeds fingerprint.MaxBits. It does not compare the
// Mapping fields; the caller checks them first. remote is expected to come
// from ParseStatus.
func NewScope(local, remote Status) (Scope, error) {
	return Scope{}, errNotImplemented
}

// CheckRange reports whether a range received from the peer may be used in
// this session (proposal §6.4): r must lie inside s.Root, and be at most
// LeafDepthSlack bits deeper than s.LeafDepth, to allow for the split of a
// range just above the leaf depth. Errors wrap ErrInvalidMessage.
func (s Scope) CheckRange(r fingerprint.Range) error {
	return errNotImplemented
}

// CheckMessage checks the parts of a reconcile Message that need no session
// state: at most MaxParts parts; every part is a Fingerprint or an Items
// part (Phase 1 rejects Symbols and More, and a part with no kind); every
// part decodes strictly (ParseFingerprint, ParseItems) and its range passes
// CheckRange. Busy and AuditRound are not checked. Errors wrap
// ErrInvalidMessage. Session.Handle runs it on every message before the
// checks that depend on the session state.
func (s Scope) CheckMessage(m *pb.Message) error {
	return errNotImplemented
}
