// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"bytes"
	"errors"
	"fmt"
	"slices"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var (
	// ErrDisjointRoots is returned by NewScope when neither root contains the
	// other: the peers' neighborhoods do not overlap and they do not sync
	// (proposal §7.6).
	ErrDisjointRoots = errors.New("rangesync: disjoint roots")
	// ErrInvalidScope is returned by NewScope when a status has Root.Bits +
	// LeafBits over fingerprint.MaxBits, or when the session root is deeper
	// than the session leaf depth.
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
	switch {
	case a.ContainsRange(b):
		return fingerprint.NewRange(b.Prefix, b.Bits), true
	case b.ContainsRange(a):
		return fingerprint.NewRange(a.Prefix, a.Bits), true
	default:
		return fingerprint.Range{}, false
	}
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
// ErrDisjointRoots when the roots do not overlap, and ErrInvalidScope when
// either status has Root.Bits + LeafBits over fingerprint.MaxBits (checked
// again, since a Status need not come from ParseStatus; the sum of two uint8
// values is computed in int, which cannot overflow) or the session root is
// deeper than the session leaf depth. It does not compare the Mapping
// fields; the caller checks them first.
//
// Because the decision rule splits only while R.Bits +
// fingerprint.FanoutBits <= LeafDepth, no range of the session is ever split
// past the leaf depth, so no further bound on LeafDepth is needed.
func NewScope(local, remote Status) (Scope, error) {
	localLeaf := int(local.Root.Bits) + int(local.LeafBits)
	remoteLeaf := int(remote.Root.Bits) + int(remote.LeafBits)
	if localLeaf > fingerprint.MaxBits || remoteLeaf > fingerprint.MaxBits {
		return Scope{}, fmt.Errorf("%w: leaf depth %d or %d over %d", ErrInvalidScope, localLeaf, remoteLeaf, fingerprint.MaxBits)
	}
	root, ok := SessionRoot(local.Root, remote.Root)
	if !ok {
		return Scope{}, fmt.Errorf("%w: %s and %s", ErrDisjointRoots, local.Root, remote.Root)
	}
	leaf := min(localLeaf, remoteLeaf)
	if int(root.Bits) > leaf {
		return Scope{}, fmt.Errorf("%w: session root %s deeper than leaf depth %d", ErrInvalidScope, root, leaf)
	}
	return Scope{Root: root, LeafDepth: uint8(leaf)}, nil
}

// CheckRange reports whether a range received from the peer may be used in
// this session (proposal §6.4): r must lie inside s.Root, and be at most
// LeafDepthSlack bits deeper than s.LeafDepth (computed in int). Splits never
// go past the leaf depth, so the slack is left only for the peer's opening
// ranges. Errors wrap ErrInvalidMessage.
func (s Scope) CheckRange(r fingerprint.Range) error {
	if !s.Root.ContainsRange(r) {
		return invalid("range %s outside session root %s", r, s.Root)
	}
	if int(r.Bits) > int(s.LeafDepth)+LeafDepthSlack {
		return invalid("range %s deeper than leaf depth %d", r, s.LeafDepth)
	}
	return nil
}

// CheckMessage checks the parts of a reconcile Message that need no session
// state: at most MaxParts parts; every part is a Fingerprint or an Items
// part (Phase 1 rejects Symbols and More, and a part with no kind); every
// part decodes strictly (ParseFingerprint, ParseItems) and its range passes
// CheckRange. Busy and AuditRound are not checked. Errors wrap
// ErrInvalidMessage. Session.Handle runs it on every message before the
// checks that depend on the session state.
func (s Scope) CheckMessage(m *pb.Message) error {
	_, err := s.parseMessage(m)
	return err
}

// part is a decoded part of a reconcile Message: a Fingerprint (items nil)
// or an Items page.
type part struct {
	r       fingerprint.Range
	summary fingerprint.Summary
	items   *Items
}

// parseMessage runs the checks of CheckMessage and returns the decoded parts.
func (s Scope) parseMessage(m *pb.Message) ([]part, error) {
	if m == nil {
		return nil, invalid("missing message")
	}
	if len(m.Parts) > MaxParts {
		return nil, invalid("message of %d parts", len(m.Parts))
	}
	parts := make([]part, len(m.Parts))
	for i, p := range m.Parts {
		switch k := p.GetKind().(type) {
		case *pb.Part_FP:
			r, sum, err := ParseFingerprint(k.FP)
			if err != nil {
				return nil, fmt.Errorf("part %d: %w", i, err)
			}
			parts[i] = part{r: r, summary: sum}
		case *pb.Part_Items:
			it, err := ParseItems(k.Items)
			if err != nil {
				return nil, fmt.Errorf("part %d: %w", i, err)
			}
			parts[i] = part{r: it.Range, items: &it}
		case nil:
			return nil, invalid("part %d has no kind", i)
		default:
			return nil, invalid("part %d: %T not supported", i, k)
		}
		if err := s.CheckRange(parts[i].r); err != nil {
			return nil, fmt.Errorf("part %d: %w", i, err)
		}
	}
	return parts, nil
}

// disjoint reports whether no two of the ranges overlap. Prefix ranges are
// nested or disjoint, and sorted by their first address the ranges overlap
// if and only if two neighbors do.
func disjoint(ranges []fingerprint.Range) bool {
	keys := make([]fingerprint.RangeKey, len(ranges))
	for i, r := range ranges {
		keys[i] = r.Key()
	}
	slices.SortFunc(keys, func(a, b fingerprint.RangeKey) int {
		if c := bytes.Compare(a.Prefix[:], b.Prefix[:]); c != 0 {
			return c
		}
		return int(a.Bits) - int(b.Bits)
	})
	for i := 1; i < len(keys); i++ {
		a := fingerprint.Range{Prefix: swarm.NewAddress(keys[i-1].Prefix[:]), Bits: keys[i-1].Bits}
		b := fingerprint.Range{Prefix: swarm.NewAddress(keys[i].Prefix[:]), Bits: keys[i].Bits}
		if a.ContainsRange(b) || b.ContainsRange(a) {
			return false
		}
	}
	return true
}
