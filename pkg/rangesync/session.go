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
	// range of this peer (hint(R)).
	Hints Hints
}

// Decision is the outcome of the per-range decision rule (proposal §6.3) for
// one Fingerprint received from the peer.
//
// The numeric values are not stable: later phases may insert decisions and
// renumber the ones after them. Never send, store, log or use a Decision as a
// metric label by its integer value.
type Decision int

// Decisions in the order Decide tests them.
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
	// DecisionStripe: initiator only, the range is splittable (R.Bits +
	// fingerprint.FanoutBits <= LeafDepth) and the estimated difference
	// max(|cL − cR|, 1) exceeds LargeDiff; the range is offered to the
	// Striper and split if it is declined or there is none.
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
	// Initiator reports whether this node opened the session.
	Initiator bool
	// Cap is the range's capacity (Capacity), 0 when symbol streams are not
	// used.
	Cap uint64
	// Hint is the size of the last decoded difference of the range with
	// this peer, 0 if none.
	Hint uint64
}

// Estimate returns d̂ = max(|cL − cR|, Hint, 1) of in (proposal §6.3).
func (in DecisionInput) Estimate() uint64 {
	return max(estimate(in.Local.Count, in.Remote.Count), in.Hint)
}

// Decide applies the per-range decision rule of proposal §6.3 to in. It is a
// pure function. The order is: DecisionEqual, DecisionSendItems,
// DecisionRequestItems, DecisionStripe, DecisionSymbols, DecisionSplit,
// DecisionExchangeItems.
func Decide(in DecisionInput) Decision {
	if in.Local == in.Remote {
		return DecisionEqual
	}
	if in.Remote.Count == 0 {
		return DecisionSendItems
	}
	if in.Local.Count == 0 {
		return DecisionRequestItems
	}
	splittable := int(in.Range.Bits)+fingerprint.FanoutBits <= int(in.Scope.LeafDepth)
	d := in.Estimate()
	if splittable && in.Initiator && d > LargeDiff {
		return DecisionStripe
	}
	if n0 := FirstBatch(d); in.Cap > 0 && n0 <= in.Cap && n0 <= MaxBatchSymbols {
		return DecisionSymbols
	}
	if splittable {
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
	// peer has not answered.
	unanswered map[fingerprint.RangeKey]struct{}
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
	// served holds this side's symbol streams by range, until answered.
	served map[fingerprint.RangeKey]*served
	// decoding holds the decoders of the peer's symbol streams by range.
	decoding map[fingerprint.RangeKey]*decoding
	stats    SessionStats
}

// inStream is the state of one of the peer's Items streams.
type inStream struct {
	last         fingerprint.ID // last listed ID, set while more
	more         bool
	reply        bool
	answersReply bool   // answers this side's Reply stream for the range
	listed       uint64 // IDs listed in the stream so far
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
		unanswered: make(map[fingerprint.RangeKey]struct{}),
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
		s.unanswered[r.Key()] = struct{}{}
	}
	return s, &pb.Message{Parts: parts}, nil
}

// NewResponder returns the session of the node that serves it. The first
// Message it handles is the initiator's opening Message. The stream handler
// reads the opening Message's AuditRound before calling NewResponder, to
// choose a cfg.Local that salts its summaries for that round (Phase 5); the
// Session itself ignores AuditRound. cfg.Striper and cfg.Open are not used.
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
//     batch without symbols must be Exhausted;
//   - a More part continues this side's symbol stream for its range, whose
//     last batch was not Exhausted and is not answered yet, past the
//     symbols sent;
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
	in := DecisionInput{Scope: s.cfg.Scope, Range: r, Local: local, Remote: remote, Initiator: s.initiator}
	if s.symbols {
		in.Cap = Capacity(s.cfg.Status, s.cfg.PeerStatus, r, local.Count, remote.Count)
		if s.cfg.Hints != nil {
			in.Hint = s.cfg.Hints.Hint(r)
		}
	}
	switch Decide(in) {
	case DecisionEqual:
	case DecisionSendItems:
		s.addListing(r, false, false)
	case DecisionRequestItems:
		s.addListing(r, true, true)
	case DecisionStripe:
		if s.cfg.Striper != nil && s.cfg.Striper.Stripe(r, local.Count, remote.Count) {
			break
		}
		s.tasks = append(s.tasks, &split{r: r})
	case DecisionSymbols:
		return s.startServing(r, local.Count, FirstBatch(in.Estimate()), in.Cap)
	case DecisionSplit:
		s.tasks = append(s.tasks, &split{r: r})
	case DecisionExchangeItems:
		s.addListing(r, true, false)
	}
	return nil
}

func (s *Session) addListing(r fingerprint.Range, reply, empty bool) {
	l := &listing{r: r, reply: reply, empty: empty}
	s.tasks = append(s.tasks, l)
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
		if _, ok := s.unanswered[key]; ok {
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

// pendingReplies reports the state kept for answers to the peer's Reply
// pages: the intervals with an exclusion list, the IDs in those lists, and
// all pending intervals.
func (s *Session) pendingReplies() (withExclusion, excludedIDs, intervals int) {
	for _, a := range s.answers {
		for _, iv := range a.intervals {
			if len(iv.excl) > 0 {
				withExclusion++
			}
			excludedIDs += len(iv.excl)
		}
		intervals += len(a.intervals)
	}
	return withExclusion, excludedIDs, intervals
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
		s.unanswered[c.Key()] = struct{}{}
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
