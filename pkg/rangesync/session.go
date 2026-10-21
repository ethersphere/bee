// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"errors"
	"fmt"
	"math/bits"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
)

// ErrPeerBusy is returned by Session.Handle for a Message with Busy set: the
// responder is over its session cap, rebuilding its index or filling its
// address index, and the session ends without a result.
var ErrPeerBusy = errors.New("rangesync: peer busy")

// ErrInvalidOpening is returned by NewInitiator for a Config.Open that breaks
// its rules. It is this node's configuration error: it does not wrap
// ErrInvalidMessage and is not the peer's fault.
var ErrInvalidOpening = errors.New("rangesync: invalid opening")

// errSessionOver is returned by Handle once the session ended or failed.
var errSessionOver = errors.New("rangesync: session over")

// Summarizer returns the summary of the local elements in a range.
// *fingerprint.Index satisfies it for ranges at most its leaf bits below its
// root; for deeper ranges it returns an error and the caller scans.
type Summarizer interface {
	// Summary returns the number and the digest of the local elements in r,
	// with the zero Digest when the count is 0. r is canonical.
	Summary(r fingerprint.Range) (fingerprint.Summary, error)
}

var _ Summarizer = (*fingerprint.Index)(nil)

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
	// Symbols returns the coded symbols [start, end) of the stream of the
	// local elements in r, as fingerprint.Index.Symbols with the address
	// index as its scan path. exhausted is true when no symbols past the
	// returned ones are available. The symbols are copies the caller owns.
	// A scan that outgrows its bound under concurrent puts ends with an error.
	Symbols(r fingerprint.Range, start, end uint64) (syms []fingerprint.CodedSymbol, exhausted bool, err error)
}

// Striper takes over ranges in which this node holds too few elements to
// reconcile them in a session, for a striped transfer from several peers
// (proposal §6.3, §7.4). The reconciler implements it.
type Striper interface {
	// Stripe offers range r, in which this node holds local elements and the
	// peer remote, more than the session's LargeDiff above local. It returns
	// true if it takes r over: the session then emits nothing more for r. It
	// returns false to let the session decide on r as if the difference
	// were small: a listing at count 0, a split, or an item exchange.
	Stripe(r fingerprint.Range, local, remote uint64) bool
}

// TransferStriper is a Striper that knows the ranges this node is already
// transferring striped. In a range such a transfer covers, the session's
// threshold of DecisionStripe is 0 (DecisionInput.Transferring): the lacking
// side declines the range whatever it lacks, so that a peer lists nothing
// the transfer lists, also in the transfer's tail, where the difference is
// below LargeDiff (T4.7 part 1, AC-B2). The reconciler implements it.
type TransferStriper interface {
	Striper
	// Transferring reports whether a striped transfer of this node covers
	// r.
	Transferring(r fingerprint.Range) bool
}

// Config configures a Session.
type Config struct {
	// Scope is the session scope, from NewScope.
	Scope Scope
	// Local reads the node's elements.
	Local Local
	// Striper, if not nil, is offered the ranges in which this node holds
	// fewer elements than the peer by more than LargeDiff (DecisionStripe),
	// on either side of the session: the lacking side takes them out of the
	// session, and the holder never lists them to it (T4.7 part 1, AC-B2).
	Striper Striper
	// LargeDiff is the threshold of DecisionStripe; zero means LargeDiff.
	// Other values are for tests that scale the geometry of a bootstrap.
	LargeDiff uint64
	// Open lists the ranges whose Fingerprints form the opening Message of an
	// initiator: the whole scope by default (nil means {Scope.Root}), a
	// stripe, or the two halves of a new root. The ranges must pass
	// Scope.CheckRange, be pairwise disjoint and number at most MaxParts. It
	// is not used by responders.
	Open []fingerprint.Range
	// PeerCount is the element count of the peer's last Status. It bounds
	// the IDs the peer may list in one range (see MaxPeerItems). Zero means
	// unknown: only the cR of the range and MaxPeerItems bound it.
	PeerCount uint64
	// Status is this node's Status and PeerStatus the peer's last Status.
	// They give the capacity of the ranges' symbol streams (Capacity). The
	// session uses symbol streams only when both Mappings are non-zero and
	// equal to Mapper's ID (Reconcile checks it before opening; Handle checks
	// it again before decoding); otherwise it reconciles in the Phase-1 form
	// (splits and item exchanges) and Symbols parts from the peer are a
	// violation, or an error wrapping ErrMappingMismatch when the Mappings
	// differ.
	Status, PeerStatus Status
	// Mapper maps elements to coded symbols; nil means
	// fingerprint.FloatMapper.
	Mapper fingerprint.Mapper
	// Hints, if not nil, keeps the size of the last decoded difference per
	// range of this peer (hint(R)): recorded by both sides of a symbol
	// stream, the decoding side and the serving side, and read by the side
	// that decides on the peer's Fingerprint.
	Hints Hints
}

// Decision is the outcome of the per-range decision rule (proposal §6.3) for
// one Fingerprint received from the peer.
//
// The numeric values are not stable: later phases may insert decisions and
// renumber the ones after them. Never send, store, log or use a Decision as a
// metric label by its integer value.
type Decision int

// Decisions in the order Decide tests them, except DecisionStripe, which
// comes second.
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
	// DecisionStripe: the range is splittable (R.Bits +
	// fingerprint.FanoutBits <= LeafDepth) and this node holds fewer
	// elements than the peer by more than the threshold (cR − cL >
	// DecisionInput.LargeDiff, or cR > cL where Transferring), in either
	// role; the range is offered to the Striper, and decided by the rest of
	// the rule if it is declined or there is none.
	DecisionStripe
	// DecisionSymbols: the first batch of FirstBatch(d̂) symbols, with d̂ =
	// max(|cL − cR|, Hint, 1), fits in the range's capacity (Cap) and in
	// MaxBatchSymbols; Symbols{R, 0 … n0} is emitted.
	DecisionSymbols
	// DecisionSplit: the range is splittable (R.Bits +
	// fingerprint.FanoutBits <= LeafDepth); the Fingerprints of its
	// 1<<fingerprint.FanoutBits children (R.Children(fingerprint.FanoutBits))
	// are emitted together. Children are therefore never deeper than the
	// session leaf depth.
	DecisionSplit
	// DecisionExchangeItems: the range is not splittable (R.Bits +
	// fingerprint.FanoutBits > LeafDepth, which includes every range at or
	// below the leaf depth); the local IDs in it are emitted as Items pages
	// with Reply.
	DecisionExchangeItems
)

// DecisionInput holds the inputs of the decision rule for one range.
type DecisionInput struct {
	// Scope is the session scope.
	Scope Scope
	// Range is the range of the received Fingerprint; it passed
	// Scope.CheckRange.
	Range fingerprint.Range
	// Local is this node's summary of Range; Remote is the peer's.
	Local, Remote fingerprint.Summary
	// Initiator reports whether this node opened the session. Decide does
	// not read it: since T4.7 part 1 the rule is the same in both roles.
	Initiator bool
	// Cap is the range's capacity (Capacity), 0 when symbol streams are not
	// used.
	Cap uint64
	// Hint is the size of the last decoded difference of the range with
	// this peer, 0 if none.
	Hint uint64
	// LargeDiff is the threshold of DecisionStripe; zero means LargeDiff.
	LargeDiff uint64
	// Transferring reports that a striped transfer of this node covers the
	// range (TransferStriper): the threshold of DecisionStripe is then 0.
	Transferring bool
}

// Estimate returns d̂ = max(|cL − cR|, Hint, 1) of in (proposal §6.3).
func (in DecisionInput) Estimate() uint64 {
	return max(estimate(in.Local.Count, in.Remote.Count), in.Hint)
}

// Decide applies the per-range decision rule of proposal §6.3 to in. It is a
// pure function. The order is: DecisionEqual, DecisionStripe,
// DecisionSendItems, DecisionRequestItems, DecisionSymbols, DecisionSplit,
// DecisionExchangeItems.
//
// The large-difference test comes before the count-0 branches and acts on
// the side that holds fewer, the initiator or the responder (candidate A of
// AC-B2, T4.7 part 1): that side neither asks for the peer's items nor
// splits, but hands the range to its Striper, and the holder decides as for
// a small difference, so that its listing at the lacking side's count 0
// (DecisionSendItems) still serves the stripe sessions, which the lacking
// side opens at count 0 (§7.4). This deviates from §6.3's order, where the
// count-0 branches come first and the initiator stripes in either direction.
func Decide(in DecisionInput) Decision {
	if in.Local == in.Remote {
		return DecisionEqual
	}
	if in.lacks() {
		return DecisionStripe
	}
	return in.decideSmall()
}

// lacks reports whether the range is splittable and this node holds fewer
// elements than the peer by more than the threshold (DecisionStripe).
func (in DecisionInput) lacks() bool {
	threshold := in.LargeDiff
	switch {
	case in.Transferring:
		threshold = 0
	case threshold == 0:
		threshold = LargeDiff
	}
	return in.splittable() && in.Remote.Count > in.Local.Count && in.Remote.Count-in.Local.Count > threshold
}

// splittable reports whether the range may be split: R.Bits +
// fingerprint.FanoutBits <= LeafDepth.
func (in DecisionInput) splittable() bool {
	return int(in.Range.Bits)+fingerprint.FanoutBits <= int(in.Scope.LeafDepth)
}

// decideSmall is Decide past DecisionStripe, for differing summaries: the
// decision of a range the lacking side does not stripe.
func (in DecisionInput) decideSmall() Decision {
	if in.Remote.Count == 0 {
		return DecisionSendItems
	}
	if in.Local.Count == 0 {
		return DecisionRequestItems
	}
	if n0 := FirstBatch(in.Estimate()); in.Cap > 0 && n0 <= in.Cap && n0 <= MaxBatchSymbols {
		return DecisionSymbols
	}
	if in.splittable() {
		return DecisionSplit
	}
	return DecisionExchangeItems
}

// estimate returns the count estimate of the difference between two counts:
// max(|a − b|, 1).
func estimate(a, b uint64) uint64 {
	if a < b {
		a, b = b, a
	}
	return max(a-b, 1)
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
// at most MaxMessageBytes. Work that does not fit is deferred to later
// Messages: whole splits (all children of a range in one Message), Items
// pages of this side's streams, and answers to the peer's Reply pages. Items
// pages carry at most ItemsPageSize IDs and fewer when the Message is nearly
// full. A side never sends an empty Message while it has work pending, or
// while one of its Items streams has More set, unless that stream answers
// the peer's Reply stream and waits for the peer's next Reply page.
//
// Deferred work is kept small: a deferred split is its range, and the IDs of
// an Items stream are read lazily from Local.IterateIDs with a cursor when
// the page is emitted, never ahead of time. An answer to a peer's Reply page
// is kept as the interval the page covers plus the IDs of the page that this
// node holds, which it must not send back. While a Message is handled every
// Reply page keeps those IDs (bounded by the size of the received Message);
// once the returned Message is built, at most MaxPendingReplies of the
// answers still pending keep them. The others are kept as their interval
// only, merged with neighboring intervals of the same range that have no
// list either, and answered with every local ID in the interval: the peer
// receives IDs it already holds, which it does not want, and the memory
// stays bounded by the number of ranges.
//
// Unanswered ranges are bounded by the scope: splits stop at the leaf depth,
// so with LeafDepth − Scope.Root.Bits <= fingerprint.LeafBits an honest
// responder holds at most 16 + 4096 unanswered ranges and an initiator
// opening with the root at most 1 + 256.
//
// A Session is not safe for concurrent use.
type Session struct {
	cfg       Config
	initiator bool
	received  bool // a Message was handled
	over      bool // the session ended or failed

	// unanswered holds the ranges this side sent a Fingerprint for that the
	// peer has not answered, with the count the Fingerprint carried (0 for
	// a range this side serves a symbol stream for).
	unanswered map[fingerprint.RangeKey]uint64
	// peerCounts holds the peer's count of each range it sent a
	// Fingerprint for.
	peerCounts map[fingerprint.RangeKey]uint64
	// in holds the peer's Items streams by range, ended ones included.
	in map[fingerprint.RangeKey]*inStream
	// replies holds this side's Items streams with Reply by range.
	replies map[fingerprint.RangeKey]*listing
	// answers holds this side's answers to the peer's Reply streams by range.
	answers map[fingerprint.RangeKey]*answer
	// tasks is the work to emit, in order.
	tasks []task

	mapper  fingerprint.Mapper
	symbols bool // symbol streams are used
	// symbolsSent and symbolsReceived count the coded symbols of all
	// streams of the session, each at most SessionSymbolBudget.
	symbolsSent, symbolsReceived uint64
	// startScans counts the symbol streams started by the received Message
	// that need the scan path, at most MaxScansPerMessage.
	startScans int
	// served holds this side's symbol streams by range, until answered.
	served map[fingerprint.RangeKey]*served
	// decoding holds the decoders of the peer's symbol streams by range.
	decoding map[fingerprint.RangeKey]*decoding
	stats    SessionStats
	// decoded counts the IDs of the peer's symbol streams this side
	// decoded, of both directions, and listings holds every listing of this
	// side (Difference).
	decoded  uint64
	listings []*listing
}

// inStream is the state of one of the peer's Items streams.
type inStream struct {
	last         fingerprint.ID // last listed ID, set while more
	more         bool
	reply        bool
	answersReply bool   // answers this side's Reply stream for the range
	listed       uint64 // IDs listed in the stream so far
	wants        uint64 // listed IDs this node does not hold
	// For a stream that answers this side's Fingerprint or symbol stream:
	// localCount is this side's count of the range (from the Fingerprint or
	// the stream); decoded reports that the peer decoded this side's symbol
	// stream and lists the IDs only it holds, and peerCount and symbols are
	// then the count of the peer's Fingerprint of the range and the symbols
	// this side sent for it.
	localCount uint64
	peerCount  uint64
	symbols    uint64
	decoded    bool
}

func newSession(cfg Config, initiator bool) *Session {
	m := cfg.Mapper
	if m == nil {
		m = fingerprint.NewFloatMapper()
	}
	return &Session{
		mapper:     m,
		symbols:    cfg.Status.Mapping != 0 && cfg.Status.Mapping == m.ID() && cfg.PeerStatus.Mapping == m.ID(),
		served:     make(map[fingerprint.RangeKey]*served),
		decoding:   make(map[fingerprint.RangeKey]*decoding),
		cfg:        cfg,
		initiator:  initiator,
		unanswered: make(map[fingerprint.RangeKey]uint64),
		in:         make(map[fingerprint.RangeKey]*inStream),
		replies:    make(map[fingerprint.RangeKey]*listing),
		answers:    make(map[fingerprint.RangeKey]*answer),
		peerCounts: make(map[fingerprint.RangeKey]uint64),
	}
}

// itemLimit returns the most IDs the peer may list in the range of key:
// max(cR, PeerCount) + ItemsPageSize of slack for elements the peer gained
// since it sent them, at most MaxPeerItems. cR is the peer's count of the
// range from its Fingerprint, if it sent one. With neither known, it is
// MaxPeerItems.
func (s *Session) itemLimit(key fingerprint.RangeKey) uint64 {
	cR, ok := s.peerCounts[key]
	if !ok && s.cfg.PeerCount == 0 {
		return MaxPeerItems
	}
	return min(max(cR, s.cfg.PeerCount)+ItemsPageSize, MaxPeerItems)
}

// NewInitiator returns the session of the node that opens it, and the opening
// Message: the Fingerprints of cfg.Open. It returns an error if cfg.Open
// breaks its rules, or if Local fails. The caller sets the opening Message's
// AuditRound for a salted session (Phase 5), with a cfg.Local that salts its
// summaries accordingly.
func NewInitiator(cfg Config) (*Session, *pb.Message, error) {
	open := cfg.Open
	if len(open) == 0 {
		open = []fingerprint.Range{cfg.Scope.Root}
	}
	if len(open) > MaxParts {
		return nil, nil, fmt.Errorf("%w: %d ranges", ErrInvalidOpening, len(open))
	}
	for _, r := range open {
		// not wrapped: a bad opening is this node's error, not a violation
		if cfg.Scope.CheckRange(r) != nil {
			return nil, nil, fmt.Errorf("%w: range %s outside the scope or too deep", ErrInvalidOpening, r)
		}
	}
	if !disjoint(open) {
		return nil, nil, fmt.Errorf("%w: overlapping ranges", ErrInvalidOpening)
	}
	s := newSession(cfg, true)
	parts := make([]*pb.Part, 0, len(open))
	for _, r := range open {
		r = fingerprint.NewRange(r.Prefix, r.Bits)
		sum, err := cfg.Local.Summary(r)
		if err != nil {
			return nil, nil, fmt.Errorf("local summary of %s: %w", r, err)
		}
		parts = append(parts, fingerprintPart(r, sum))
		s.unanswered[r.Key()] = sum.Count
	}
	return s, &pb.Message{Parts: parts}, nil
}

// NewResponder returns the session of the node that serves it. The first
// Message it handles is the initiator's opening Message. The stream handler
// reads the opening Message's AuditRound before calling NewResponder, to
// choose a cfg.Local that salts its summaries for that round (Phase 5); the
// Session itself ignores AuditRound. cfg.Open is not used.
func NewResponder(cfg Config) *Session {
	return newSession(cfg, false)
}

// Handle processes Message in from the peer and returns the Message to send
// back, and the IDs the peer listed that this node does not hold (wants), for
// the fetch stream. A nil out with a nil error means that the session is
// over: in was empty and this side has nothing pending, so nothing is sent.
// An out with no parts is the empty Message.
//
// The caller should read in with a reader limited to MaxMessageBytes; Handle
// also rejects a larger in as a violation, since the state kept for the
// peer's Reply pages while a Message is handled is bounded by its size.
//
// Each Fingerprint part is answered by the decision rule (Decide). A Symbols
// part is fed to the decoder of its range (see Config.Status for when
// symbol streams are used): once decoded, the IDs only the peer holds are
// added to wants and the IDs only this node holds are sent as Items without
// Reply; while not decoded a More is sent, until the stream is Exhausted or
// reaches the range's Capacity, when the range is split (or its items
// exchanged with Reply at the leaf depth); a corrupt stream falls back to an
// item exchange with Reply (Stats counts it). Each side's stream is taken
// from one snapshot of its elements: its cached prefix is copied when it
// starts, and later symbols are used only while their first symbols equal
// it; a stream whose elements changed ends there (Exhausted). Each Items
// part adds the listed IDs that Local does not hold to wants; with Reply set
// it is answered with the local IDs in the interval the page covers that the
// page does not list (see Items), as Items pages without Reply.
//
// This side sends at most SessionSymbolBudget symbols in a session (the
// batch that reaches it is Exhausted, and no range gets a symbol stream
// once it sent or received that many). Per Message it starts at most
// MaxScansPerMessage symbol streams that need the scan path (the other
// ranges take the Fingerprint or items path) and emits at most
// MaxScansPerMessage batches that need it (the others wait for a later
// Message).
//
// The work of one call is bounded: at most one Summary per received
// Fingerprint and one per emitted Fingerprint (each at most MaxParts), one
// HasID per received ID, and IterateIDs reads only as far as the IDs the
// returned Message carries plus the IDs it skips because a Reply page listed
// them: those of the received Message, and at most MaxPendingReplies ×
// ItemsPageSize kept from earlier Messages.
//
// A Message with Busy set is valid only as the first Message an initiator
// handles, and only with no parts; Handle then returns ErrPeerBusy and the
// session ends. A non-zero in.AuditRound is ignored. Every violation of the
// rules below by the peer returns an error wrapping ErrInvalidMessage, after
// which the session must not be used:
//
//   - in passes Scope.CheckMessage;
//   - Busy is set only as described above;
//   - the responder's first Message is the opening: one or more Fingerprint
//     parts and nothing else, of pairwise disjoint ranges;
//   - after the opening, a Fingerprint part belongs to the split of a range
//     R this side sent a Fingerprint for and the peer has not answered yet,
//     with R.Bits + fingerprint.FanoutBits <= Scope.LeafDepth (a split the
//     decision rule makes): exactly R.Children(fingerprint.FanoutBits), each
//     once, in the same Message, which answers R;
//   - an Items part with an empty After starts the peer's stream for its
//     range, which answers either a range this side sent a Fingerprint for
//     and the peer has not answered yet, or, without Reply, a range for
//     which this side sent Items pages with Reply; there is at most one
//     peer stream per range;
//   - an Items part with After set continues the peer's stream for its
//     range, whose last page had More set, and After equals the last ID the
//     stream listed;
//   - the peer's stream that answers this side's Reply pages for a range
//     ends (a page without More) only after this side sent its last Reply
//     page for that range;
//   - a Symbols part with Start 0 starts the peer's symbol stream for its
//     range, which answers a range this side sent a Fingerprint for and the
//     peer has not answered yet; a later batch must continue the stream
//     (Start equals the symbols received) and answer this side's More, up
//     to its UpTo; the data must parse (fingerprint.ParseSymbolData) and a
//     batch without symbols must be Exhausted; its Count is at least 1, at
//     most the peer's item limit and the same in every batch of the stream;
//     the stream stays within cap(R) (computed from the count this side
//     sent), which must not be 0; all streams of the session together carry
//     at most SessionSymbolBudget symbols;
//   - a More part continues this side's symbol stream for its range, whose
//     last batch was not Exhausted and is not answered yet, past the
//     symbols sent and up to at most cap(R);
//   - the peer sends an empty Message only when none of its Items streams
//     has More set, except a stream that answers this side's Reply stream
//     for a range whose last page this side has not sent yet: the peer is
//     waiting for that page.
//
// A range is answered at most once. The peer may answer a Fingerprint in
// any later Message, since work that does not fit is deferred; a range it
// never answers was equal at its end.
//
// An error from Local (for example the Index being busy with a rebuild) is
// returned wrapped, does not wrap ErrInvalidMessage and ends the session; it
// is not the peer's fault and must not be scored.
func (s *Session) Handle(in *pb.Message) (out *pb.Message, wants []fingerprint.ID, err error) {
	if s.over {
		return nil, nil, errSessionOver
	}
	out, wants, err = s.handle(in)
	if err != nil || out == nil {
		s.over = true
	}
	if err != nil {
		return nil, nil, err
	}
	return out, wants, nil
}

// Difference returns n_Δ of the session so far: the number of elements only
// one of the two sides holds, in the ranges the session resolved (proposal
// §10, pairwise_diff). Ranges taken over by the Striper, and ranges the peer
// never answered (equal), count nothing. Both sides of a completed session
// return the same value when neither side's elements changed during it.
//
// Each resolved range adds:
//   - a symbol stream of the peer that this side decoded: the IDs it
//     recovered, both directions;
//   - a symbol stream of this side that the peer decoded: the IDs the peer
//     listed (those only it holds) plus the local IDs the peer lacks, which
//     follow from the two counts: |only local| = cL − (cR − listed), within
//     [0, cL], where cL is the count of this side's stream and cR the count
//     of the peer's Fingerprint;
//   - otherwise, this side's wants in the range (the IDs the peer listed
//     that this node does not hold) plus |only local| = cL − (cR − wants),
//     within [0, cL]. cL is this side's count of the range (its listing or
//     Fingerprint) and cR the peer's (its complete listing, or the count of
//     its Fingerprint).
//
// Only the first is exact under churn: it is the size of the difference
// between the two snapshots the streams were taken from. The others combine
// counts and listings from different times (the peer's Fingerprint is older
// than the snapshot it decoded against, and wants miss the IDs this node
// gained since), so elements that changed during the session make them
// approximate: for a stream of this side that the peer decoded, the error is
// the peer's change between its Fingerprint and the snapshot it decoded
// against. It is a metric and nothing acts on it. A peer that lies about its
// counts changes it within the local count of each range.
func (s *Session) Difference() uint64 {
	n := s.decoded
	for _, l := range s.listings {
		var wants uint64
		if st := s.in[l.r.Key()]; st != nil && st.answersReply {
			wants = st.wants
		}
		n += wants + onlyLocal(l.listed, l.peerCount, wants)
	}
	for _, st := range s.in {
		if st.answersReply {
			continue // counted with its listing above
		}
		if st.decoded {
			n += st.decodedDifference()
			continue
		}
		// a complete listing: SendItems, Exchange, the fallbacks
		n += st.wants + onlyLocal(st.localCount, st.listed, st.wants)
	}
	return n
}

// decodedDifference returns n_Δ of a range whose symbol stream (this side's)
// the peer decoded, from the peer's listing of the IDs only it holds: the
// listed IDs plus the local IDs the peer lacks, local − (peer − listed). It
// counts the listing, not this side's wants, which miss the listed IDs this
// node gained since it served the stream (from another peer).
func (st *inStream) decodedDifference() uint64 {
	return st.listed + onlyLocal(st.localCount, st.peerCount, st.listed)
}

// servedHint returns the hint(R) the serving side of a decoded symbol stream
// records once the peer's listing ended: the difference (decodedDifference),
// at most the symbols this side sent. Without it the serving side, which in
// the steady state never decodes (the lower overlay initiates and decodes the
// higher's root stream), would start every stream at MinBatch and grow it by
// GrowthFactor. The bound holds for an honest peer, whose decoder recovers at
// most one ID per symbol received, and keeps a peer that lists more IDs or
// lies about its count from raising this node's next first batch past its
// last stream, which the peer asked for within cap(R) and
// SessionSymbolBudget: no more than a Fingerprint whose count differs from
// this node's can ask for.
func (st *inStream) servedHint() uint64 {
	return min(st.decodedDifference(), st.symbols)
}

// onlyLocal returns the number of this side's elements of a range that the
// peer lacks: local − (peer − wants), within [0, local], where local and
// peer are the two sides' counts of the range and wants the number of the
// peer's elements this side lacks.
func onlyLocal(local, peer, wants uint64) uint64 {
	common := peer - min(wants, peer)
	if common >= local {
		return 0
	}
	return local - common
}

func (s *Session) handle(in *pb.Message) (*pb.Message, []fingerprint.ID, error) {
	first := !s.received
	s.received = true
	if in == nil {
		return nil, nil, invalid("missing message")
	}
	if in.Busy {
		if s.initiator && first && len(in.Parts) == 0 {
			return nil, nil, ErrPeerBusy
		}
		return nil, nil, invalid("unexpected busy")
	}
	parts, err := s.cfg.Scope.parseMessage(in)
	if err != nil {
		return nil, nil, err
	}

	opening := !s.initiator && first
	if opening {
		if err := checkOpening(parts); err != nil {
			return nil, nil, err
		}
	}
	if len(parts) == 0 {
		for key, st := range s.in {
			// a stream answering our unfinished Reply stream waits for its
			// next page, which this side still owes
			waiting := st.answersReply && !s.replies[key].done
			if st.more && !waiting {
				return nil, nil, invalid("empty message with an open items stream")
			}
		}
		if !s.pending() {
			return nil, nil, nil
		}
	}

	var wants []fingerprint.ID
	// splits holds, for each range the peer split in this message, the
	// children seen so far as a bit mask.
	splits := make(map[fingerprint.RangeKey]uint64)
	s.startScans = 0
	for _, p := range parts {
		switch {
		case p.items != nil:
			w, err := s.handleItems(*p.items)
			if err != nil {
				return nil, nil, err
			}
			wants = append(wants, w...)
			continue
		case p.symbols != nil:
			w, err := s.handleSymbols(*p.symbols)
			if err != nil {
				return nil, nil, err
			}
			wants = append(wants, w...)
			continue
		case p.more != nil:
			if err := s.handleMore(*p.more); err != nil {
				return nil, nil, err
			}
			continue
		}
		if !opening {
			if err := s.checkChild(p.r, splits); err != nil {
				return nil, nil, err
			}
		}
		if err := s.decide(p.r, p.summary); err != nil {
			return nil, nil, err
		}
	}
	for _, mask := range splits {
		if mask != 1<<splitParts-1 {
			return nil, nil, invalid("incomplete split")
		}
	}

	out, err := s.build()
	if err != nil {
		return nil, nil, err
	}
	return out, wants, nil
}

// checkOpening checks the responder's first Message: one or more
// Fingerprints of pairwise disjoint ranges and nothing else.
func checkOpening(parts []part) error {
	if len(parts) == 0 {
		return invalid("empty opening")
	}
	ranges := make([]fingerprint.Range, len(parts))
	for i, p := range parts {
		if p.items != nil || p.symbols != nil || p.more != nil {
			return invalid("%s in the opening", p.kind())
		}
		ranges[i] = p.r
	}
	if !disjoint(ranges) {
		return invalid("overlapping ranges in the opening")
	}
	return nil
}

// checkChild checks that the Fingerprint of r belongs to the peer's split of
// a range this side sent and the peer has not answered, which r's split in
// this message answers. splits records the children seen per parent.
func (s *Session) checkChild(r fingerprint.Range, splits map[fingerprint.RangeKey]uint64) error {
	if r.Bits < fingerprint.FanoutBits {
		return invalid("fingerprint %s is no child of a split", r)
	}
	parent := fingerprint.NewRange(r.Prefix, r.Bits-fingerprint.FanoutBits)
	if int(parent.Bits)+fingerprint.FanoutBits > int(s.cfg.Scope.LeafDepth) {
		return invalid("split of %s past leaf depth %d", parent, s.cfg.Scope.LeafDepth)
	}
	key := parent.Key()
	mask, seen := splits[key]
	if !seen {
		if _, ok := s.unanswered[key]; !ok {
			return invalid("split of %s, which is not an unanswered range", parent)
		}
		delete(s.unanswered, key)
		s.closeServed(key)
	}
	bit := uint64(1) << parent.ChildIndex(r.Prefix, fingerprint.FanoutBits)
	if mask&bit != 0 {
		return invalid("child %s twice", r)
	}
	splits[key] = mask | bit
	return nil
}

// decide answers the peer's Fingerprint of r with summary remote.
func (s *Session) decide(r fingerprint.Range, remote fingerprint.Summary) error {
	local, err := s.cfg.Local.Summary(r)
	if err != nil {
		return fmt.Errorf("local summary of %s: %w", r, err)
	}
	s.peerCounts[r.Key()] = remote.Count
	in := DecisionInput{Scope: s.cfg.Scope, Range: r, Local: local, Remote: remote, Initiator: s.initiator, LargeDiff: s.cfg.LargeDiff}
	// No symbol stream once the budget is spent either way, nor past
	// MaxScansPerMessage starts that scan (each reads at least symbol 0):
	// those ranges take the Fingerprint or items path.
	scan := cacheCapacity(s.cfg.Status, r) == 0
	if s.symbols && s.symbolsSent < SessionSymbolBudget && s.symbolsReceived < SessionSymbolBudget &&
		(!scan || s.startScans < MaxScansPerMessage) {
		in.Cap = Capacity(s.cfg.Status, s.cfg.PeerStatus, r, local.Count, remote.Count)
		if s.cfg.Hints != nil {
			in.Hint = s.cfg.Hints.Hint(r)
		}
	}
	if ts, ok := s.cfg.Striper.(TransferStriper); ok && remote.Count > local.Count {
		in.Transferring = ts.Transferring(r)
	}
	d := Decide(in)
	if d == DecisionStripe {
		if s.cfg.Striper != nil && s.cfg.Striper.Stripe(r, local.Count, remote.Count) {
			// taken over: nothing is emitted for r, which the peer then
			// sees as equal at the end of the session (a responder's
			// answer is the empty one of DecisionEqual)
			return nil
		}
		d = in.decideSmall()
	}
	switch d {
	case DecisionEqual:
	case DecisionSendItems:
		s.addListing(r, false, false, remote.Count)
	case DecisionRequestItems:
		s.addListing(r, true, true, remote.Count)
	case DecisionSymbols:
		if scan {
			s.startScans++
		}
		return s.startServing(r, local.Count, FirstBatch(in.Estimate()), in.Cap)
	case DecisionSplit:
		s.tasks = append(s.tasks, &split{r: r})
	case DecisionExchangeItems:
		s.addListing(r, true, false, remote.Count)
	}
	return nil
}

// addListing queues the listing of r, whose element count on the peer's side
// is peerCount.
func (s *Session) addListing(r fingerprint.Range, reply, empty bool, peerCount uint64) {
	l := &listing{r: r, reply: reply, empty: empty, peerCount: peerCount}
	s.tasks = append(s.tasks, l)
	s.listings = append(s.listings, l)
	if reply {
		s.replies[r.Key()] = l
	}
}

// handleItems checks one of the peer's Items pages against the session state
// and returns the listed IDs this node does not hold. A Reply page queues
// its answer.
func (s *Session) handleItems(it Items) ([]fingerprint.ID, error) {
	key := it.Range.Key()
	st := s.in[key]
	if it.After == nil {
		if st != nil {
			return nil, invalid("second items stream for %s", it.Range)
		}
		st = &inStream{reply: it.Reply}
		if count, ok := s.unanswered[key]; ok {
			st.localCount = count
			if sv := s.served[key]; sv != nil {
				st.localCount = sv.count
				st.decoded = !it.Reply
				st.peerCount = s.peerCounts[key]
				st.symbols = sv.sent
			}
			delete(s.unanswered, key)
			s.closeServed(key)
		} else if s.replies[key] != nil && !it.Reply {
			st.answersReply = true
		} else {
			return nil, invalid("unsolicited items for %s", it.Range)
		}
		s.in[key] = st
	} else {
		if st == nil || !st.more || *it.After != st.last {
			return nil, invalid("items for %s do not continue a stream", it.Range)
		}
		if it.Reply != st.reply {
			return nil, invalid("items for %s change the reply flag", it.Range)
		}
	}
	if st.answersReply && !it.More && !s.replies[key].done {
		return nil, invalid("answers for %s end before the last reply page", it.Range)
	}
	st.more = it.More
	st.listed += uint64(len(it.IDs))
	if limit := s.itemLimit(key); st.listed > limit {
		return nil, invalid("items for %s list more than %d ids", it.Range, limit)
	}
	if len(it.IDs) > 0 {
		st.last = it.IDs[len(it.IDs)-1]
	}
	if st.decoded && !st.more && s.cfg.Hints != nil {
		s.cfg.Hints.SetHint(it.Range, st.servedHint())
	}

	var wants, held []fingerprint.ID
	for _, id := range it.IDs {
		has, err := s.cfg.Local.HasID(id)
		if err != nil {
			return nil, fmt.Errorf("local lookup: %w", err)
		}
		switch {
		case !has:
			wants = append(wants, id)
		case it.Reply:
			held = append(held, id)
		}
	}
	st.wants += uint64(len(wants))
	if it.Reply {
		s.addReplyInterval(it, held)
	}
	return wants, nil
}

// addReplyInterval queues the answer to the peer's Reply page it, of which
// this node holds the IDs held.
func (s *Session) addReplyInterval(it Items, held []fingerprint.ID) {
	key := it.Range.Key()
	a := s.answers[key]
	if a == nil {
		a = &answer{r: it.Range}
		s.answers[key] = a
		s.tasks = append(s.tasks, a)
	}
	if !it.More {
		a.peerDone = true
	}
	iv := &interval{lo: it.After}
	if it.More {
		hi := it.IDs[len(it.IDs)-1]
		iv.hi = &hi
	}
	if len(held) > 0 {
		iv.excl = make(map[fingerprint.ID]struct{}, len(held))
		for _, id := range held {
			iv.excl[id] = struct{}{}
		}
	}
	// Consecutive intervals of a stream are contiguous; merge the ones
	// without an exclusion list.
	if n := len(a.intervals); n > 0 && len(iv.excl) == 0 && len(a.intervals[n-1].excl) == 0 {
		a.intervals[n-1].hi = iv.hi
		return
	}
	a.intervals = append(a.intervals, iv)
}

// pending reports whether this side has work to send.
func (s *Session) pending() bool {
	for _, t := range s.tasks {
		if t.pending() {
			return true
		}
	}
	return false
}

// build emits as much of the pending work as fits in one Message.
func (s *Session) build() (*pb.Message, error) {
	b := new(builder)
	kept := s.tasks[:0]
	for _, t := range s.tasks {
		finished, err := t.emit(s, b)
		if err != nil {
			return nil, err
		}
		if !finished {
			kept = append(kept, t)
		}
	}
	clear(s.tasks[len(kept):])
	s.tasks = kept
	s.capExclusions()
	return &pb.Message{Parts: b.parts}, nil
}

// capExclusions keeps the exclusion lists of the first MaxPendingReplies
// pending reply intervals, in answer order, drops the others and merges the
// neighboring intervals of an answer that have no list.
func (s *Session) capExclusions() {
	kept := 0
	for _, t := range s.tasks {
		a, ok := t.(*answer)
		if !ok {
			continue
		}
		merged := a.intervals[:0]
		for _, iv := range a.intervals {
			if len(iv.excl) > 0 {
				if kept < MaxPendingReplies {
					kept++
				} else {
					iv.excl = nil
				}
			}
			if n := len(merged); n > 0 && len(iv.excl) == 0 && len(merged[n-1].excl) == 0 {
				merged[n-1].hi = iv.hi
				continue
			}
			merged = append(merged, iv)
		}
		clear(a.intervals[len(merged):])
		a.intervals = merged
	}
}

// Upper bounds of the encoded size of the parts a Session emits, including
// the framing of the part inside its Message (tag and length varint, at most
// 1 + 3 bytes, once for the Part in the Message and once for the kind in the
// Part).
const (
	// fingerprintWireBytes bounds a Fingerprint part: a range of at most
	// 2 + 34 + 3 bytes, a count of at most 11 and a digest of 34.
	fingerprintWireBytes = 88
	// pageWireBytes bounds an Items part without IDs: a range of at most 39
	// bytes, a cursor of 66 and two flags of 2.
	pageWireBytes = 128
	// idWireBytes is the size of one ID in an Items part: tag, length and
	// the 2 × (2 + 32) byte message.
	idWireBytes = 70
	// splitParts is the number of children of a split.
	splitParts = 1 << fingerprint.FanoutBits
)

// builder collects the parts of one outgoing Message within MaxParts and
// MaxMessageBytes.
type builder struct {
	parts []*pb.Part
	size  int // encoded size of the Message so far
	scans int // Symbols parts produced by the scan path
}

func (b *builder) fits(parts, size int) bool {
	return len(b.parts)+parts <= MaxParts && b.size+size <= MaxMessageBytes
}

func (b *builder) add(p *pb.Part) {
	n := p.Size()
	b.size += 1 + (bits.Len64(uint64(n)|1)+6)/7 + n
	b.parts = append(b.parts, p)
}

// pageRoom returns the number of IDs a new Items page can carry, or -1 when
// not even an empty page fits.
func (b *builder) pageRoom() int {
	if !b.fits(1, pageWireBytes) {
		return -1
	}
	return min(ItemsPageSize, (MaxMessageBytes-b.size-pageWireBytes)/idWireBytes)
}

func fingerprintPart(r fingerprint.Range, s fingerprint.Summary) *pb.Part {
	return &pb.Part{Kind: &pb.Part_FP{FP: EncodeFingerprint(r, s)}}
}

func pagePart(it Items) *pb.Part {
	return &pb.Part{Kind: &pb.Part_Items{Items: EncodeItems(it)}}
}

// task is deferrable outgoing work.
type task interface {
	// pending reports whether the task has something to send now.
	pending() bool
	// emit adds what fits to b and reports whether the task is finished.
	emit(s *Session, b *builder) (finished bool, err error)
}

// split emits the Fingerprints of the children of r, all together.
type split struct {
	r fingerprint.Range
}

func (*split) pending() bool { return true }

func (t *split) emit(s *Session, b *builder) (bool, error) {
	if !b.fits(splitParts, splitParts*fingerprintWireBytes) {
		return false, nil
	}
	for _, c := range t.r.Children(fingerprint.FanoutBits) {
		sum, err := s.cfg.Local.Summary(c)
		if err != nil {
			return false, fmt.Errorf("local summary of %s: %w", c, err)
		}
		b.add(fingerprintPart(c, sum))
		s.unanswered[c.Key()] = sum.Count
	}
	return true, nil
}

// listing is an Items stream of every local ID in r, or, when empty, one
// page without IDs (a request for the peer's IDs).
type listing struct {
	r      fingerprint.Range
	reply  bool
	empty  bool
	cursor *fingerprint.ID
	done   bool
	// listed counts the IDs emitted so far; peerCount is the peer's count
	// of r (Difference).
	listed    uint64
	peerCount uint64
}

func (l *listing) pending() bool { return !l.done }

func (l *listing) emit(s *Session, b *builder) (bool, error) {
	for !l.done {
		room := b.pageRoom()
		if room < 0 || room == 0 && !l.empty {
			return false, nil
		}
		page := Items{Range: l.r, After: l.cursor, Reply: l.reply}
		if !l.empty {
			ids, more, err := s.list(l.r, l.cursor, room)
			if err != nil {
				return false, err
			}
			page.IDs, page.More = ids, more
		}
		b.add(pagePart(page))
		l.listed += uint64(len(page.IDs))
		if n := len(page.IDs); n > 0 {
			last := page.IDs[n-1]
			l.cursor = &last
		}
		l.done = !page.More
	}
	return true, nil
}

// list returns up to limit IDs of local elements in r after after, and
// whether more follow.
func (s *Session) list(r fingerprint.Range, after *fingerprint.ID, limit int) ([]fingerprint.ID, bool, error) {
	var ids []fingerprint.ID
	more := false
	err := s.cfg.Local.IterateIDs(r, after, func(id fingerprint.ID) (bool, error) {
		if len(ids) == limit {
			more = true
			return true, nil
		}
		ids = append(ids, id)
		return false, nil
	})
	if err != nil {
		return nil, false, fmt.Errorf("local ids of %s: %w", r, err)
	}
	return ids, more, nil
}

// interval is a pending part of an answer: the local IDs after lo (the scan
// position, nil for the start of the range) up to and including hi (nil for
// the end of the range), except those in excl.
type interval struct {
	lo, hi *fingerprint.ID
	excl   map[fingerprint.ID]struct{}
}

// answer is this side's Items stream answering the peer's Reply stream for
// r.
type answer struct {
	r         fingerprint.Range
	intervals []*interval
	cursor    *fingerprint.ID
	started   bool
	ended     bool
	peerDone  bool // the peer's Reply stream ended
}

func (a *answer) pending() bool {
	return len(a.intervals) > 0 || a.peerDone && a.started && !a.ended
}

func (a *answer) emit(s *Session, b *builder) (bool, error) {
	for {
		if len(a.intervals) == 0 {
			if !a.peerDone {
				return false, nil // waits for the peer's next Reply page
			}
			if !a.started {
				a.ended = true
				return true, nil
			}
			if b.pageRoom() < 0 {
				return false, nil
			}
			b.add(pagePart(Items{Range: a.r, After: a.cursor}))
			a.ended = true
			return true, nil
		}
		room := b.pageRoom()
		if room < 1 {
			return false, nil
		}
		ids, err := a.collect(s, room)
		if err != nil {
			return false, err
		}
		if len(ids) == 0 {
			continue // the intervals held nothing to send
		}
		more := len(a.intervals) > 0 || !a.peerDone
		b.add(pagePart(Items{Range: a.r, IDs: ids, After: a.cursor, More: more}))
		last := ids[len(ids)-1]
		a.cursor, a.started = &last, true
		if !more {
			a.ended = true
			return true, nil
		}
	}
}

// collect reads up to limit IDs from the pending intervals, dropping the
// intervals it exhausts.
func (a *answer) collect(s *Session, limit int) ([]fingerprint.ID, error) {
	var ids []fingerprint.ID
	for len(a.intervals) > 0 && len(ids) < limit {
		iv := a.intervals[0]
		full := false
		err := s.cfg.Local.IterateIDs(a.r, iv.lo, func(id fingerprint.ID) (bool, error) {
			if iv.hi != nil && id.Compare(*iv.hi) > 0 {
				return true, nil
			}
			if len(ids) == limit {
				full = true
				return true, nil
			}
			pos := id
			iv.lo = &pos
			if _, ok := iv.excl[id]; !ok {
				ids = append(ids, id)
			}
			return false, nil
		})
		if err != nil {
			return nil, fmt.Errorf("local ids of %s: %w", a.r, err)
		}
		if full {
			break
		}
		a.intervals[0] = nil
		a.intervals = a.intervals[1:]
	}
	return ids, nil
}
