// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"errors"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
)

// ErrPeerBusy is returned by Session.Handle for a Message with Busy set: the
// responder is over its session cap, rebuilding its index or filling its
// address index, and the session ends without a result.
var ErrPeerBusy = errors.New("rangesync: peer busy")

// Summarizer returns the summary of the local elements in a range.
// *fingerprint.Index satisfies it for ranges at most its leaf bits below its
// root; for deeper ranges it returns an error and the caller scans.
type Summarizer interface {
	// Summary returns the number and the digest of the local elements in r,
	// with the zero Digest when the count is 0. r is canonical.
	Summary(r fingerprint.Range) (fingerprint.Summary, error)
}

// Local is the local side of a session: the node's elements, read through
// the fingerprint index and the storer's address index. The stream handlers
// build it from *fingerprint.Index and the storer (ReserveIterateAddr,
// ReserveHasID), answering summaries of ranges below the index leaf depth
// with a scan. The ranges a Session passes lie inside its scope, which lies
// inside the local root, and are at most LeafDepthSlack bits deeper than the
// session leaf depth. An error from Local ends the session; it does not wrap
// ErrInvalidMessage unless the peer caused it.
type Local interface {
	Summarizer
	// IterateIDs calls fn with the IDs of the local elements in r in ascending
	// order, starting strictly after after, or at the start of r when after
	// is nil, until fn returns stop true or an error, which IterateIDs
	// returns.
	IterateIDs(r fingerprint.Range, after *fingerprint.ID, fn func(fingerprint.ID) (stop bool, err error)) error
	// HasID reports whether the local reserve holds the element id.
	HasID(id fingerprint.ID) (bool, error)
}

// Striper takes over ranges whose estimated difference is too large to
// reconcile in a session, for a striped transfer from several peers
// (proposal §6.3, §7.4). It is implemented by the reconciler in Phase 3; until
// then sessions run without one.
type Striper interface {
	// Stripe offers range r, in which this node holds local elements and the
	// peer remote. It returns true if it takes r over: the session then emits
	// nothing more for r. It returns false to let the session split r.
	Stripe(r fingerprint.Range, local, remote uint64) bool
}

// Config configures a Session.
type Config struct {
	// Scope is the session scope, from NewScope.
	Scope Scope
	// Local reads the node's elements.
	Local Local
	// Striper, if not nil, is offered ranges with a large difference. It is
	// used by initiators only; responders split such ranges.
	Striper Striper
}

// Decision is the outcome of the per-range decision rule (proposal §6.3) for
// one Fingerprint received from the peer.
type Decision int

// Decisions in the order Decide tests them. The Phase-2 symbol stream ("fits
// in symbols") is decided after DecisionStripe and before
// DecisionExchangeItems.
const (
	// DecisionEqual: both summaries are equal and the range is reconciled;
	// nothing is emitted.
	DecisionEqual Decision = iota + 1
	// DecisionSendItems: the peer has no element in the range (cR == 0); all
	// local IDs in it are emitted as Items pages without Reply.
	DecisionSendItems
	// DecisionRequestItems: this node has no element in the range (cL == 0);
	// Items{R, no IDs, Reply} is emitted and the peer answers with its IDs.
	DecisionRequestItems
	// DecisionStripe: initiator only, the range is above the session leaf
	// depth and the estimated difference max(|cL − cR|, 1) exceeds LargeDiff;
	// the range is offered to the Striper and split if it is declined or
	// there is none.
	DecisionStripe
	// DecisionExchangeItems: the range is at least as deep as the session
	// leaf depth (R.Bits >= LeafDepth); the local IDs in it are emitted as
	// Items pages with Reply.
	DecisionExchangeItems
	// DecisionSplit: the range is split; the Fingerprints of its
	// 1<<fingerprint.FanoutBits children (R.Children(fingerprint.FanoutBits))
	// are emitted together.
	DecisionSplit
)

// Decide applies the per-range decision rule of proposal §6.3, in its Phase-1
// form, to range r of scope, in which this node's summary is local and the
// peer's is remote. initiator reports whether this node opened the session.
// It is a pure function. r must have passed scope.CheckRange.
func Decide(scope Scope, r fingerprint.Range, local, remote fingerprint.Summary, initiator bool) Decision {
	return 0
}

// Session is the decision logic of one reconciliation session (proposal §6.3,
// §6.4) between this node and one peer. It does no network I/O: the stream
// handlers feed it the Messages they read and send the Messages it returns.
// It reads local summaries and IDs through Config.Local.
//
// A session alternates Messages, starting with the initiator's opening
// Message. Each side answers every received Message with exactly one Message
// until it has nothing to send; then it sends an empty Message. A side that
// receives an empty Message while it has nothing pending ends the session
// without answering. So an empty Message is sent only by a side with nothing
// pending, and the side that sent it reads either the peer's next Message or
// the end of the stream, both normal.
//
// Every Message the session returns has at most MaxParts parts and encodes to
// at most MaxMessageBytes. Work that does not fit is kept for later Messages:
// whole splits (all children of a range in one Message) and Items pages,
// which carry at most ItemsPageSize IDs and fewer when the Message is nearly
// full. A side never sends an empty Message while it has work pending or an
// Items stream with More set.
//
// A Session is not safe for concurrent use.
type Session struct {
	cfg       Config
	initiator bool
}

// NewInitiator returns the session of the node that opens it, and the opening
// Message: the Fingerprint of the session root.
func NewInitiator(cfg Config) (*Session, *pb.Message, error) {
	return &Session{cfg: cfg, initiator: true}, nil, errNotImplemented
}

// NewResponder returns the session of the node that serves it. The first
// Message it handles is the initiator's opening Message. cfg.Striper is not
// used.
func NewResponder(cfg Config) *Session {
	return &Session{cfg: cfg}
}

// Handle processes Message in from the peer and returns the Message to send
// back, and the IDs the peer listed that this node does not hold (wants), for
// the fetch stream. A nil out with a nil error means that the session is
// over: in was empty and this side has nothing pending, so nothing is sent.
// An out with no parts is the empty Message.
//
// Each Fingerprint part is answered by the decision rule (Decide). Each Items
// part adds the listed IDs that Local does not hold to wants; with Reply set
// it is answered with the local IDs in the interval the page covers that the
// page does not list (see Items), as Items pages without Reply.
//
// Handle returns ErrPeerBusy if in.Busy is set. A non-zero in.AuditRound is
// ignored in Phase 1. Every violation of the rules below by the peer returns
// an error wrapping ErrInvalidMessage, after which the session must not be
// used:
//
//   - in passes Scope.CheckMessage;
//   - the responder's first Message is the opening: one or more Fingerprint
//     parts and nothing else, of pairwise disjoint ranges;
//   - after the opening, a Fingerprint part belongs to the split of a range
//     this side sent a Fingerprint for and the peer has not answered yet:
//     all 1<<fingerprint.FanoutBits children of that range, each once, in
//     the same Message, which answers the range;
//   - an Items part with an empty After starts the peer's stream for its
//     range, which answers either a range this side sent a Fingerprint for
//     and the peer has not answered yet, or, without Reply, a range for
//     which this side sent Items pages with Reply; there is at most one
//     peer stream per range;
//   - an Items part with After set continues the peer's stream for its
//     range, whose last page had More set, and After equals the last ID the
//     stream listed;
//   - the peer sends an empty Message only when none of its Items streams
//     has More set.
//
// The peer may answer a Fingerprint in any later Message, since work that
// does not fit is deferred; a range it never answers was equal at its end.
// Errors from Local are returned wrapped and end the session.
func (s *Session) Handle(in *pb.Message) (out *pb.Message, wants []fingerprint.ID, err error) {
	return nil, nil, errNotImplemented
}
