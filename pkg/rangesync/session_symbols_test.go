// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"errors"
	"math"
	"slices"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestFirstBatch(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ d, want uint64 }{
		{0, rangesync.MinBatch},
		{1, rangesync.MinBatch},
		{2, 6},
		{3, 7},
		{10, 18},
		{250, 402},
		{2500, 4002},
		{4000, 6402},
		{math.MaxUint64, (16*(math.MaxUint64/32)+9)/10 + 2}, // clamped, never overflows
	} {
		if got := rangesync.FirstBatch(tc.d); got != tc.want {
			t.Errorf("FirstBatch(%d) = %d, want %d", tc.d, got, tc.want)
		}
	}
}

func TestCapacity(t *testing.T) {
	t.Parallel()

	whole := status(rangeOf(""), 12)
	edge := status(rangeOf("0110"), 12)
	big := uint64(fingerprint.ScanLimit + 1)
	for _, tc := range []struct {
		name          string
		local, remote rangesync.Status
		r             fingerprint.Range
		cL, cR        uint64
		want          uint64
	}{
		{"root of both", whole, whole, rangeOf(""), big, big, fingerprint.RootSymbols},
		{"level-1 of both", whole, whole, rangeOf("0110"), big, big, fingerprint.Level1Symbols},
		{"bucket", whole, whole, rangeOf("01101"), big, big, fingerprint.BucketSymbols},
		{"leaf", whole, whole, rangeOf("011011001100"), big, big, fingerprint.BucketSymbols},
		{"small on both", whole, whole, rangeOf("01101"), 10, fingerprint.ScanLimit, rangesync.CapUnlimited},
		{"small on one: its cache bounds the other", whole, whole, rangeOf(""), 10, big, fingerprint.RootSymbols},
		{"root of one, level-1 of the other", whole, edge, rangeOf("0110"), big, big, fingerprint.Level1Symbols},
		{"edge root small on the edge peer", edge, whole, rangeOf("0110"), 3, big, fingerprint.Level1Symbols},
		{"below a peer's leaf depth", status(rangeOf(""), 4), whole, rangeOf("01101"), big, big, 0},
	} {
		if got := rangesync.Capacity(tc.local, tc.remote, tc.r, tc.cL, tc.cR); got != tc.want {
			t.Errorf("%s: Capacity = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestDecideSymbols(t *testing.T) {
	t.Parallel()

	scope := rangesync.Scope{Root: rangeOf(""), LeafDepth: 12}
	root, leaf := rangeOf(""), rangeOf("011011001100")
	for _, tc := range []struct {
		name      string
		r         fingerprint.Range
		cL, cR    uint64
		capR      uint64
		hint      uint64
		initiator bool
		want      rangesync.Decision
	}{
		{"no symbols: split", root, 100, 90, 0, 0, false, rangesync.DecisionSplit},
		{"no symbols at the leaf: items", leaf, 100, 90, 0, 0, false, rangesync.DecisionExchangeItems},
		{"fits the root cache", root, 100, 90, fingerprint.RootSymbols, 0, false, rangesync.DecisionSymbols},
		{"equal counts: MinBatch", root, 100, 100, fingerprint.BucketSymbols, 0, false, rangesync.DecisionSymbols},
		{"just fits", root, 2600, 100, fingerprint.RootSymbols, 0, false, rangesync.DecisionSymbols}, // 4002
		{"too large for the root cache", root, 4000, 100, fingerprint.RootSymbols, 0, false, rangesync.DecisionSplit},
		{"hint raises the batch past the cap", root, 100, 100, fingerprint.BucketSymbols, 30, false, rangesync.DecisionSplit},
		{"hint within the cap", root, 100, 100, fingerprint.BucketSymbols, 10, false, rangesync.DecisionSymbols},
		{"unlimited leaf", leaf, 100, 3, rangesync.CapUnlimited, 0, false, rangesync.DecisionSymbols},
		{"unlimited, over a batch", root, 1 << 20, 1, rangesync.CapUnlimited, 0, false, rangesync.DecisionSplit},
		{"stripe first", root, 1 << 20, 1, rangesync.CapUnlimited, 0, true, rangesync.DecisionStripe},
		{"empty peer: items", root, 100, 0, fingerprint.RootSymbols, 0, false, rangesync.DecisionSendItems},
	} {
		in := rangesync.DecisionInput{
			Scope: scope, Range: tc.r, Initiator: tc.initiator, Cap: tc.capR, Hint: tc.hint,
			Local: sum(tc.cL, 1), Remote: sum(tc.cR, 2),
		}
		if tc.cR == 0 {
			in.Remote = fingerprint.Summary{}
		}
		if got := rangesync.Decide(in); got != tc.want {
			t.Errorf("%s: Decide = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// symbolsPair returns an initiator and a responder over la and lb that use
// symbol streams, both with Status st, and the opening Message.
func symbolsPair(t *testing.T, st rangesync.Status, la, lb *fakeLocal, open ...fingerprint.Range) (a, b *rangesync.Session, first *pb.Message) {
	t.Helper()
	scope := mustScope(t, st, st)
	la.status, lb.status = &st, &st
	a, first, err := rangesync.NewInitiator(rangesync.Config{Scope: scope, Local: la, Status: st, PeerStatus: st, Open: open})
	if err != nil {
		t.Fatal(err)
	}
	b = rangesync.NewResponder(rangesync.Config{Scope: scope, Local: lb, Status: st, PeerStatus: st})
	return a, b, first
}

// partKinds records, per message, the kinds of its parts.
type partKinds struct {
	msgs [][]string
}

func (k *partKinds) inspect(fromInitiator bool, m *pb.Message) {
	var kinds []string
	side := "B:"
	if fromInitiator {
		side = "A:"
	}
	for _, p := range m.GetParts() {
		switch {
		case p.GetFP() != nil:
			kinds = append(kinds, side+"fp")
		case p.GetSymbols() != nil:
			kinds = append(kinds, side+"symbols")
		case p.GetMore() != nil:
			kinds = append(kinds, side+"more")
		case p.GetItems().GetReply():
			kinds = append(kinds, side+"items-reply")
		default:
			kinds = append(kinds, side+"items")
		}
	}
	k.msgs = append(k.msgs, kinds)
}

func (k *partKinds) count(kind string) int {
	n := 0
	for _, m := range k.msgs {
		for _, x := range m {
			if x == kind {
				n++
			}
		}
	}
	return n
}

// TestSessionSymbolsStaleStamp is the stale-stamp divergence of proposal
// §7.7: both sides hold chunk c, under different stamps. The symbol stream
// recovers both IDs and each side learns the other's.
func TestSessionSymbolsStaleStamp(t *testing.T) {
	t.Parallel()

	rng := newRand(77)
	root := rangeOf("")
	common := randIDs(rng, root, 3000)
	c1 := randID(rng, root)
	stamp := make([]byte, swarm.HashSize)
	stamp[0] = 1
	c2 := fingerprint.NewID(c1.Address(), stamp)
	la := newFakeLocal(append(slices.Clone(common), c1)...)
	lb := newFakeLocal(append(slices.Clone(common), c2)...)
	a, b, first := symbolsPair(t, status(root, 12), la, lb)
	var kinds partKinds
	wantsA, wantsB, messages := drive(t, a, first, b, kinds.inspect)
	assertSet(t, wantsA, []fingerprint.ID{c2})
	assertSet(t, wantsB, []fingerprint.ID{c1})
	// FP(root); Symbols(root) (and More while it does not decode);
	// Items(root, [c1]); empty
	if kinds.count("B:symbols") == 0 || kinds.count("B:fp") != 0 || kinds.count("A:items-reply") != 0 {
		t.Fatalf("%d messages %v, want FP, Symbols, Items, empty", messages, kinds.msgs)
	}
	if a.Stats().Decoded != 1 {
		t.Fatalf("stats %+v", a.Stats())
	}
}

// TestSessionSymbolsExhausted checks that a stream that ends before it
// decodes is split above the leaf depth and exchanged as items at it, and
// that the session still converges.
func TestSessionSymbolsExhausted(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		open     fingerprint.Range
		wantNext string // the initiator's answer to the exhausted stream
	}{
		{"above the leaf depth: split", rangeOf(""), "A:fp"},
		{"at the leaf depth: items", rangeOf("011011001100"), "A:items-reply"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rng := newRand(5)
			scope := rangesync.Scope{Root: rangeOf(""), LeafDepth: 12}
			la, lb, a2b, b2a := convergenceSets(rng, tc.open, scope, 300, 20, 20, false)
			lb.symbolLimit = 4 // the responder's streams end after MinBatch symbols
			a, b, first := symbolsPair(t, status(rangeOf(""), 12), la, lb, tc.open)
			var kinds partKinds
			wantsA, wantsB, _ := drive(t, a, first, b, kinds.inspect)
			assertSet(t, wantsA, a2b)
			assertSet(t, wantsB, b2a)
			if a.Stats().Exhausted == 0 {
				t.Fatalf("no exhausted stream: %+v", a.Stats())
			}
			if len(kinds.msgs) < 3 || !slices.Equal(kinds.msgs[1], []string{"B:symbols"}) || !slices.Contains(kinds.msgs[2], tc.wantNext) {
				t.Fatalf("messages %v, want Symbols answered with %s", kinds.msgs[:min(3, len(kinds.msgs))], tc.wantNext)
			}
			if slices.Contains(kinds.msgs[2], "A:more") {
				t.Fatal("more for an exhausted stream")
			}
		})
	}
}

// TestSessionSymbolsCorrupt feeds the initiator a stream that holds an ID
// outside the range. Decoding reports corruption, the range falls back to
// an exact item exchange with Reply, and no element is lost.
func TestSessionSymbolsCorrupt(t *testing.T) {
	t.Parallel()

	rng := newRand(9)
	root := rangeOf("01")
	scope := rangesync.Scope{Root: root, LeafDepth: 14}
	la, lb, a2b, b2a := convergenceSets(rng, root, scope, 500, 5, 5, false)
	lb.extra = []fingerprint.ID{randID(rng, rangeOf("1"))}
	a, b, first := symbolsPair(t, status(root, 12), la, lb)
	var kinds partKinds
	wantsA, wantsB, _ := drive(t, a, first, b, kinds.inspect)
	assertSet(t, wantsA, a2b)
	assertSet(t, wantsB, b2a)
	if got := a.Stats().DecodeCorruptions; got != 1 {
		t.Fatalf("%d corruptions, want 1", got)
	}
	if kinds.count("A:items-reply") == 0 {
		t.Fatalf("no item exchange after the corruption: %v", kinds.msgs)
	}
}

// TestSessionSymbolsSnapshot changes one side's elements while its stream
// is being served or decoded. Each stream comes from one snapshot: symbols
// past the cached prefix are only used if the elements did not change, so an
// honest stream never decodes corrupt; a changed one ends (Exhausted) and the
// range is split, and the session converges on the differences that did not
// change.
func TestSessionSymbolsSnapshot(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// churn changes the initiator (true) or the responder (false), if set
		churn, initiator bool
	}{
		{name: "no churn"},
		{name: "responder churn", churn: true},
		{name: "initiator churn", churn: true, initiator: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rng := newRand(11)
			// a bucket-depth range: a cached prefix of 32 symbols, the scan
			// path past it
			r := rangeOf("01101100")
			scope := rangesync.Scope{Root: rangeOf(""), LeafDepth: 12}
			la, lb, a2b, b2a := convergenceSets(rng, r, scope, 500, 20, 20, false)
			a, b, first := symbolsPair(t, status(rangeOf(""), 12), la, lb, r)
			changed := la
			if !tc.churn || !tc.initiator {
				changed = lb
			}
			added := randID(rng, r)
			n := 0
			churn := func(bool, *pb.Message) {
				if n++; tc.churn && n == 3 { // after the first batch was decoded
					changed.ids = append(changed.ids, added)
					slices.SortFunc(changed.ids, fingerprint.ID.Compare)
				}
			}
			var kinds partKinds
			wantsA, wantsB, _ := drive(t, a, first, b, churn, kinds.inspect)
			if c := a.Stats().DecodeCorruptions + b.Stats().DecodeCorruptions; c != 0 {
				t.Fatalf("%d corrupt decodes", c)
			}
			if kinds.count("A:more") < 2 {
				t.Fatalf("stream did not pass its cached prefix: %v", kinds.msgs)
			}
			switch {
			case !tc.churn:
				assertSet(t, wantsA, a2b)
				assertSet(t, wantsB, b2a)
				if st := a.Stats(); st.Exhausted != 0 || st.Decoded != 1 {
					t.Fatalf("stats %+v, want one decoded stream", st)
				}
			case tc.initiator:
				assertSet(t, wantsA, a2b)
				assertSet(t, wantsB, append(slices.Clone(b2a), added))
			default:
				assertSet(t, wantsA, append(slices.Clone(a2b), added))
				assertSet(t, wantsB, b2a)
			}
			if tc.churn && a.Stats().Exhausted == 0 {
				t.Fatalf("stats %+v, want an exhausted stream", a.Stats())
			}
		})
	}
}

// TestSessionSymbolsHints checks that a decoded difference is recorded and
// raises the next session's first batch.
func TestSessionSymbolsHints(t *testing.T) {
	t.Parallel()

	rng := newRand(13)
	root := rangeOf("")
	scope := rangesync.Scope{Root: root, LeafDepth: 12}
	st := status(root, 12)
	la, lb, _, _ := convergenceSets(rng, root, scope, 300, 10, 10, true)
	hints := mapHints{}
	firstBatch := func(hints rangesync.Hints) int {
		la.status, lb.status = &st, &st
		a, first, err := rangesync.NewInitiator(rangesync.Config{Scope: scope, Local: la, Status: st, PeerStatus: st, Hints: hints})
		if err != nil {
			t.Fatal(err)
		}
		b := rangesync.NewResponder(rangesync.Config{Scope: scope, Local: lb, Status: st, PeerStatus: st, Hints: hints})
		n := -1
		drive(t, a, first, b, func(_ bool, m *pb.Message) {
			for _, p := range m.GetParts() {
				if p.GetSymbols() == nil || n >= 0 {
					continue
				}
				sy, err := rangesync.ParseSymbols(p.GetSymbols())
				if err != nil {
					t.Fatal(err)
				}
				syms, err := fingerprint.ParseSymbolData(fingerprint.NewFloatMapper(), sy.Count, sy.Start, sy.Data)
				if err != nil {
					t.Fatal(err)
				}
				n = len(syms)
			}
		})
		return n
	}
	if n := firstBatch(hints); n != rangesync.MinBatch {
		t.Fatalf("first batch %d, want %d", n, rangesync.MinBatch)
	}
	if hints[root.Key()] != 20 {
		t.Fatalf("hint %d, want 20", hints[root.Key()])
	}
	if n := firstBatch(hints); n != int(rangesync.FirstBatch(20)) {
		t.Fatalf("first batch %d, want %d", n, rangesync.FirstBatch(20))
	}
}

type mapHints map[fingerprint.RangeKey]uint64

func (h mapHints) Hint(r fingerprint.Range) uint64       { return h[r.Key()] }
func (h mapHints) SetHint(r fingerprint.Range, d uint64) { h[r.Key()] = d }

// TestSessionSymbolsValidation checks the session rules for Symbols and
// More parts.
func TestSessionSymbolsValidation(t *testing.T) {
	t.Parallel()

	rng := newRand(15)
	root := rangeOf("01")
	st := status(root, 12)
	scope := mustScope(t, st, st)
	local := newFakeLocal(randIDs(rng, root, 50)...)
	m := fingerprint.NewFloatMapper()
	batch := func(r fingerprint.Range, start, n uint64, exhausted bool) *pb.Part {
		syms := make([]fingerprint.CodedSymbol, n)
		for _, id := range randIDs(rng, r, 3) {
			fingerprint.Encode(m, syms, start, id, 1)
		}
		return &pb.Part{Kind: &pb.Part_Symbols{Symbols: rangesync.EncodeSymbols(rangesync.Symbols{
			Range: r, Count: 3, Start: start, Data: fingerprint.AppendSymbolData(nil, m, 3, start, syms), Exhausted: exhausted,
		})}}
	}
	morePart := func(r fingerprint.Range, upTo uint64) *pb.Part {
		return &pb.Part{Kind: &pb.Part_More{More: rangesync.EncodeMore(rangesync.More{Range: r, UpTo: upTo})}}
	}
	initiator := func(cfg rangesync.Config) *rangesync.Session {
		cfg.Scope, cfg.Local = scope, local
		s, _, err := rangesync.NewInitiator(cfg)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	symbolsCfg := rangesync.Config{Status: st, PeerStatus: st}
	// serving returns a responder that served the first batch of root.
	serving := func() *rangesync.Session {
		s := rangesync.NewResponder(rangesync.Config{Scope: scope, Local: local, Status: st, PeerStatus: st})
		out, _, err := s.Handle(msg(fpPart(root, sum(53, 1))))
		if err != nil || len(out.Parts) != 1 || out.Parts[0].GetSymbols() == nil {
			t.Fatalf("no symbols: %v %v", out, err)
		}
		return s
	}
	for _, tc := range []struct {
		name     string
		session  func() *rangesync.Session
		msgs     []*pb.Message
		mismatch bool // ErrMappingMismatch instead of a violation
	}{
		{"symbols without statuses", func() *rangesync.Session { return initiator(rangesync.Config{}) }, []*pb.Message{msg(batch(root, 0, 4, false))}, false},
		{"symbols with another mapping", func() *rangesync.Session {
			return initiator(rangesync.Config{Status: st, PeerStatus: rangesync.Status{Root: root, LeafBits: 12, Mapping: 2}})
		}, []*pb.Message{msg(batch(root, 0, 4, false))}, true},
		{"unsolicited symbols", func() *rangesync.Session { return initiator(symbolsCfg) }, []*pb.Message{msg(batch(rangeOf("0110"), 0, 4, false))}, false},
		{"first batch not at 0", func() *rangesync.Session { return initiator(symbolsCfg) }, []*pb.Message{msg(batch(root, 4, 4, false))}, false},
		{"empty batch", func() *rangesync.Session { return initiator(symbolsCfg) }, []*pb.Message{msg(batch(root, 0, 0, false))}, false},
		{"bad symbol data", func() *rangesync.Session { return initiator(symbolsCfg) }, []*pb.Message{msg(&pb.Part{Kind: &pb.Part_Symbols{Symbols: &pb.Symbols{Range: rangesync.EncodeRange(root), Data: make([]byte, 82)}}})}, false},
		{"second batch without more", func() *rangesync.Session { return initiator(symbolsCfg) }, []*pb.Message{msg(batch(root, 0, 4, false), batch(root, 4, 4, false))}, false},
		{"batch past the request", func() *rangesync.Session { return initiator(symbolsCfg) }, []*pb.Message{msg(batch(root, 0, 4, false)), msg(batch(root, 4, 100, false))}, false},
		{"more for no stream", func() *rangesync.Session { return initiator(symbolsCfg) }, []*pb.Message{msg(morePart(root, 16))}, false},
		{"more twice", serving, []*pb.Message{msg(morePart(root, 16), morePart(root, 64))}, false},
		{"more not past the stream", serving, []*pb.Message{msg(morePart(root, 4))}, false},
		{"more after the stream was answered", serving, []*pb.Message{msg(itemsPart(rangesync.Items{Range: root}), morePart(root, 16))}, false},
		{"symbols in the opening", func() *rangesync.Session {
			return rangesync.NewResponder(rangesync.Config{Scope: scope, Local: local, Status: st, PeerStatus: st})
		}, []*pb.Message{msg(fpPart(root, sum(53, 1)), batch(root, 0, 4, false))}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := tc.session()
			var err error
			for _, m := range tc.msgs {
				if _, _, err = s.Handle(wire(t, m)); err != nil {
					break
				}
			}
			switch {
			case tc.mismatch:
				if !errors.Is(err, rangesync.ErrMappingMismatch) || isInvalid(err) {
					t.Fatalf("got %v, want a mapping mismatch", err)
				}
			case !isInvalid(err):
				t.Fatalf("got %v, want invalid", err)
			}
		})
	}
}
