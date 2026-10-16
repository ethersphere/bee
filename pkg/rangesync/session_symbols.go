// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
)

// Hints keeps, for the ranges of one peer, the size of the last decoded
// difference (hint(R), proposal §6.3). It may forget entries at any time.
type Hints interface {
	// Hint returns the size of the last decoded difference of r, 0 if none.
	Hint(r fingerprint.Range) uint64
	// SetHint records d as the size of the difference decoded for r.
	SetHint(r fingerprint.Range, d uint64)
}

// CapUnlimited is the Capacity of a range whose stream has no fixed end on
// either side: both peers hold at most fingerprint.ScanLimit elements in it.
const CapUnlimited = math.MaxUint64

// peerCapacity returns the number of coded symbols a peer with Status st
// that holds count elements in r can produce for r (proposal §4.3.5):
// unlimited when count <= ScanLimit (the scan path), else the cache of r: the
// root, a level-1 range or a range at most LeafBits below the root; 0 for any
// other range.
func peerCapacity(st Status, r fingerprint.Range, count uint64) uint64 {
	if count <= fingerprint.ScanLimit {
		return CapUnlimited
	}
	return cacheCapacity(st, r)
}

// cacheCapacity returns the length of the cached prefix of the stream of r
// for a peer with Status st.
func cacheCapacity(st Status, r fingerprint.Range) uint64 {
	if !st.Root.ContainsRange(r) {
		return 0
	}
	switch depth := int(r.Bits) - int(st.Root.Bits); {
	case depth == 0:
		return fingerprint.RootSymbols
	case depth == fingerprint.FanoutBits:
		return fingerprint.Level1Symbols
	case depth <= int(st.LeafBits):
		return fingerprint.BucketSymbols
	default:
		return 0
	}
}

// Capacity returns cap(R) of proposal §6.3: the number of coded symbols both
// peers can produce for r, where this node has Status local and count cL in
// r and the peer Status remote and count cR. It is the smaller of the two
// peers' capacities: unlimited for a peer holding at most ScanLimit elements
// in r, otherwise RootSymbols for its root, Level1Symbols for one of its
// level-1 ranges and BucketSymbols for any other range at most its LeafBits
// below its root.
func Capacity(local, remote Status, r fingerprint.Range, cL, cR uint64) uint64 {
	return min(peerCapacity(local, r, cL), peerCapacity(remote, r, cR))
}

// FirstBatch returns the size of the first symbol batch for the estimated
// difference d: max(MinBatch, ⌈BatchFactor · d⌉ + 2).
func FirstBatch(d uint64) uint64 {
	d = min(d, math.MaxUint64/32) // no overflow below; such batches never fit
	return max(MinBatch, (16*d+9)/10+2)
}

// symbolPartWireBytes bounds a Symbols part without its data: a range of 39
// bytes, count and start of 11 each, the data's tag and length of 4, the
// flag of 2 and the part framing of 8.
const symbolPartWireBytes = 80

// moreWireBytes bounds a More part: a range of 39 bytes, UpTo of 11 and the
// part framing of 8.
const moreWireBytes = 64

// symStream is one side's coded-symbol stream of a range, taken from one
// snapshot of its elements (the Decoder's fixed-set precondition): the cached
// prefix is copied when the stream starts, and symbols past it are accepted
// only from a stream whose start equals what was taken before.
type symStream struct {
	r         fingerprint.Range
	syms      []fingerprint.CodedSymbol // symbols [0, len(syms)) of the stream
	exhausted bool                      // no symbols past syms are available
}

// newSymStream starts the stream of the local elements in r with a copy of
// its cached prefix.
func (s *Session) newSymStream(r fingerprint.Range) (*symStream, error) {
	// At least symbol 0, to which every element maps, so that a stream
	// served by the scan path (no cache, below the leaf depth) is fixed when
	// it starts, not at its first window. (window compares every later
	// window with all symbols taken so far, so a change is detected either
	// way.)
	syms, exhausted, err := s.cfg.Local.Symbols(r, 0, max(cacheCapacity(s.cfg.Status, r), 1))
	if err != nil {
		return nil, fmt.Errorf("local symbols of %s: %w", r, err)
	}
	return &symStream{r: r, syms: syms, exhausted: exhausted}, nil
}

// window returns the symbols [start, end) of the stream, or fewer when the
// stream ends before end. Symbols past the ones taken so far come from a new
// Local.Symbols call for [0, end); if its first symbols differ from the ones
// taken (the elements changed in between), the stream ends where it was.
// The returned slice must not be modified.
func (st *symStream) window(s *Session, start, end uint64) ([]fingerprint.CodedSymbol, error) {
	if end > uint64(len(st.syms)) && !st.exhausted {
		syms, exhausted, err := s.cfg.Local.Symbols(st.r, 0, end)
		if err != nil {
			return nil, fmt.Errorf("local symbols of %s: %w", st.r, err)
		}
		if len(syms) < len(st.syms) || !slices.Equal(syms[:len(st.syms)], st.syms) {
			st.exhausted = true
		} else {
			st.syms = syms
			st.exhausted = exhausted || uint64(len(syms)) < end
		}
	}
	end = min(end, uint64(len(st.syms)))
	start = min(start, end)
	return st.syms[start:end], nil
}

// scans reports whether window(_, end) reads the scan path: it needs
// symbols past the ones taken so far.
func (st *symStream) scans(end uint64) bool {
	return end > uint64(len(st.syms)) && !st.exhausted
}

// ended reports whether the stream has no symbols past end.
func (st *symStream) ended(end uint64) bool {
	return st.exhausted && end >= uint64(len(st.syms))
}

// served is this side's symbol stream of a range, answering the peer's
// Fingerprint. It is a task while a batch is due.
type served struct {
	st        *symStream
	count     uint64 // local count of the range, the Symbols.Count
	cap       uint64 // cap(R)
	sent      uint64 // symbols sent
	upTo      uint64 // end of the batch due, > sent while due
	awaiting  bool   // the last batch was sent and is not answered yet
	exhausted bool   // a batch with Exhausted was sent
	closed    bool   // the peer answered the stream
}

func (sv *served) pending() bool { return !sv.closed && sv.upTo > sv.sent }

func (sv *served) emit(s *Session, b *builder) (bool, error) {
	if !sv.pending() {
		return true, nil
	}
	// A request past MaxBatchSymbols is cut short; the peer asks again, at
	// the cost of one more round trip. Past the session budget the batch is
	// empty and Exhausted.
	n := min(sv.upTo-sv.sent, MaxBatchSymbols, SessionSymbolBudget-s.symbolsSent)
	if !b.fits(1, symbolPartWireBytes+int(n)*fingerprint.MaxSymbolSize) {
		return false, nil
	}
	if sv.st.scans(sv.sent + n) {
		if b.scans >= MaxScansPerMessage {
			return false, nil // deferred to a later Message
		}
		b.scans++
	}
	syms, err := sv.st.window(s, sv.sent, sv.sent+n)
	if err != nil {
		return false, err
	}
	end := sv.sent + uint64(len(syms))
	s.symbolsSent += uint64(len(syms))
	exhausted := sv.st.ended(end) || end >= sv.cap || s.symbolsSent >= SessionSymbolBudget
	data := fingerprint.AppendSymbolData(nil, s.mapper, sv.count, sv.sent, syms)
	b.add(&pb.Part{Kind: &pb.Part_Symbols{Symbols: EncodeSymbols(Symbols{
		Range: sv.st.r, Count: sv.count, Start: sv.sent, Data: data, Exhausted: exhausted,
	})}})
	s.unanswered[sv.st.r.Key()] = 0
	sv.sent, sv.upTo = end, 0
	sv.awaiting, sv.exhausted = true, exhausted
	return true, nil
}

// decoding is the decoding of the peer's symbol stream of a range that
// answers this side's Fingerprint.
type decoding struct {
	dec   *fingerprint.Decoder
	st    *symStream // the local stream
	count uint64     // the peer's count of the range, Symbols.Count
	cap   uint64     // cap(R), at most SessionSymbolBudget
	upTo  uint64     // the UpTo of the More sent, 0 while none is outstanding
}

// more is a More request of the stream dc.
type more struct {
	dc *decoding
	mo More
}

func (*more) pending() bool { return true }

func (t *more) emit(_ *Session, b *builder) (bool, error) {
	if !b.fits(1, moreWireBytes) {
		return false, nil
	}
	b.add(&pb.Part{Kind: &pb.Part_More{More: EncodeMore(t.mo)}})
	t.dc.upTo = t.mo.UpTo
	return true, nil
}

// idListing is an Items stream without Reply of a fixed list of IDs in r,
// ascending; with no IDs it is one empty page.
type idListing struct {
	r      fingerprint.Range
	ids    []fingerprint.ID
	cursor *fingerprint.ID
}

func (*idListing) pending() bool { return true }

func (l *idListing) emit(_ *Session, b *builder) (bool, error) {
	for {
		room := b.pageRoom()
		if room < 0 || room == 0 && len(l.ids) > 0 {
			return false, nil
		}
		n := min(room, len(l.ids))
		page := Items{Range: l.r, IDs: l.ids[:n], After: l.cursor, More: n < len(l.ids)}
		b.add(pagePart(page))
		if n > 0 {
			last := l.ids[n-1]
			l.cursor = &last
		}
		l.ids = l.ids[n:]
		if !page.More {
			return true, nil
		}
	}
}

// startServing starts this side's symbol stream of r, with local count
// count, first batch n0 and capacity capR.
func (s *Session) startServing(r fingerprint.Range, count, n0, capR uint64) error {
	st, err := s.newSymStream(r)
	if err != nil {
		return err
	}
	sv := &served{st: st, count: count, cap: capR, upTo: min(n0, capR)}
	s.served[r.Key()] = sv
	s.tasks = append(s.tasks, sv)
	return nil
}

// closeServed ends this side's symbol stream of the range of key, which the
// peer answered.
func (s *Session) closeServed(key fingerprint.RangeKey) {
	if sv := s.served[key]; sv != nil {
		sv.closed = true
		delete(s.served, key)
	}
}

// handleMore answers the peer's More request with the next batch of this
// side's stream.
func (s *Session) handleMore(mo More) error {
	sv := s.served[mo.Range.Key()]
	if sv == nil || !sv.awaiting || sv.exhausted || mo.UpTo <= sv.sent {
		return invalid("more for %s does not continue a symbol stream", mo.Range)
	}
	if mo.UpTo > sv.cap {
		return invalid("more for %s up to %d past the capacity %d", mo.Range, mo.UpTo, sv.cap)
	}
	sv.awaiting = false
	sv.upTo = mo.UpTo
	s.tasks = append(s.tasks, sv)
	return nil
}

// errNoMapping is returned for Symbols received by a session whose Statuses
// have no common Mapping.
var errNoMapping = errors.New("symbols without a common mapping")

// handleSymbols feeds the peer's batch sy to the decoder of its range and
// queues the answer: the IDs only this side holds once decoded (and returns
// the IDs only the peer holds as wants), a More while the stream can grow, a
// split or an item exchange once it cannot, and an item exchange with Reply
// if the stream is corrupt.
func (s *Session) handleSymbols(sy Symbols) ([]fingerprint.ID, error) {
	if !s.symbols {
		if s.cfg.Status.Mapping != 0 && s.cfg.PeerStatus.Mapping != 0 {
			// both sides set a Mapping and they differ: the session must not
			// have been opened (Reconcile checks it); not the peer's fault
			return nil, fmt.Errorf("%w: %w", ErrMappingMismatch, errNoMapping)
		}
		return nil, invalid("symbols for %s: %v", sy.Range, errNoMapping)
	}
	r := sy.Range
	key := r.Key()
	dc := s.decoding[key]
	switch {
	case dc == nil:
		// a range this side streams symbols for is unanswered too, but only
		// a Fingerprint of this side can be answered by a symbol stream
		sent, ok := s.unanswered[key]
		if !ok || sy.Start != 0 || s.served[key] != nil {
			return nil, invalid("unsolicited symbols for %s", r)
		}
		// The peer chose symbols by the decision rule from the count this
		// side sent and its own: a count of 0 or past what it may list is
		// a lie, and so is a range without capacity. cap(R) is computed
		// from the count sent, as the peer did, so that a More never asks
		// past the peer's cap(R).
		if limit := s.itemLimit(key); sy.Count == 0 || sy.Count > limit {
			return nil, invalid("symbols for %s with count %d outside [1, %d]", r, sy.Count, limit)
		}
		capR := min(Capacity(s.cfg.Status, s.cfg.PeerStatus, r, sent, sy.Count), SessionSymbolBudget)
		if capR == 0 {
			return nil, invalid("symbols for %s, which has no capacity", r)
		}
		delete(s.unanswered, key)
		st, err := s.newSymStream(r)
		if err != nil {
			return nil, err
		}
		dc = &decoding{dec: fingerprint.NewDecoder(s.mapper, r), st: st, count: sy.Count, cap: capR}
		s.decoding[key] = dc
	case dc.upTo == 0 || sy.Start != dc.dec.Received():
		return nil, invalid("symbols for %s do not continue the stream", r)
	case sy.Count != dc.count:
		return nil, invalid("symbols for %s change the count", r)
	}
	remote, err := fingerprint.ParseSymbolData(s.mapper, sy.Count, sy.Start, sy.Data)
	if err != nil {
		return nil, invalid("symbols for %s: %v", r, err)
	}
	n := uint64(len(remote))
	if n == 0 && !sy.Exhausted {
		return nil, invalid("empty symbol batch for %s", r)
	}
	if dc.upTo != 0 && sy.Start+n > dc.upTo {
		return nil, invalid("symbols for %s past the requested %d", r, dc.upTo)
	}
	if sy.Start+n > dc.cap {
		return nil, invalid("symbols for %s past the capacity %d", r, dc.cap)
	}
	if s.symbolsReceived += n; s.symbolsReceived > SessionSymbolBudget {
		return nil, invalid("symbols for %s past the session budget", r)
	}
	dc.upTo = 0

	local, err := dc.st.window(s, sy.Start, sy.Start+n)
	if err != nil {
		return nil, err
	}
	short := uint64(len(local)) < n // this side cannot follow the stream further
	if len(local) > 0 {
		err = dc.dec.Add(sy.Start, remote[:len(local)], local)
	}
	switch {
	case errors.Is(err, fingerprint.ErrCorrupt):
		// §9: fall back to an exact item listing of r
		s.stats.DecodeCorruptions++
		delete(s.decoding, key)
		s.addListing(r, true, false)
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("decode symbols of %s: %w", r, err)
	}

	if dc.dec.Done() {
		delete(s.decoding, key)
		onlyLocal, onlyRemote := dc.dec.Result()
		if s.cfg.Hints != nil {
			s.cfg.Hints.SetHint(r, uint64(dc.dec.Recovered()))
		}
		s.stats.Decoded++
		s.tasks = append(s.tasks, &idListing{r: r, ids: onlyLocal})
		return onlyRemote, nil
	}
	received := dc.dec.Received()
	upTo := min(GrowthFactor*received, dc.cap, received+SessionSymbolBudget-s.symbolsReceived)
	if sy.Exhausted || short || upTo <= received {
		// The stream cannot grow: split r, or exchange its items at the
		// leaf depth. The received symbols are discarded.
		delete(s.decoding, key)
		s.stats.Exhausted++
		if s.splittable(r) {
			s.tasks = append(s.tasks, &split{r: r})
		} else {
			s.addListing(r, true, false)
		}
		return nil, nil
	}
	s.tasks = append(s.tasks, &more{dc: dc, mo: More{Range: r, UpTo: upTo}})
	return nil, nil
}

// splittable reports whether the decision rule may split r.
func (s *Session) splittable(r fingerprint.Range) bool {
	return int(r.Bits)+fingerprint.FanoutBits <= int(s.cfg.Scope.LeafDepth)
}

// SessionStats counts the symbol-stream outcomes of a session.
type SessionStats struct {
	// Decoded counts the peer's symbol streams decoded completely.
	Decoded int
	// Exhausted counts the peer's symbol streams that could not grow before
	// they decoded (the range is split or its items exchanged).
	Exhausted int
	// DecodeCorruptions counts the peer's symbol streams the decoder found
	// corrupt (proposal §6.4, §9); their ranges fall back to an item exchange.
	DecodeCorruptions int
}

// Stats returns the symbol-stream counts of the session so far.
func (s *Session) Stats() SessionStats {
	return s.stats
}
