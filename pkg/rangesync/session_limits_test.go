// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
)

func limitMore(r fingerprint.Range, upTo uint64) *pb.Part {
	return &pb.Part{Kind: &pb.Part_More{More: rangesync.EncodeMore(rangesync.More{Range: r, UpTo: upTo})}}
}

// limitBatch returns a Symbols part of n symbols of r from start with
// count, encoding a few random IDs of r.
func limitBatch(seed uint64, r fingerprint.Range, count, start, n uint64) *pb.Part {
	m := fingerprint.NewFloatMapper()
	syms := make([]fingerprint.CodedSymbol, n)
	for _, id := range randIDs(newRand(seed), r, 3) {
		fingerprint.Encode(m, syms, start, id, 1)
	}
	return &pb.Part{Kind: &pb.Part_Symbols{Symbols: rangesync.EncodeSymbols(rangesync.Symbols{
		Range: r, Count: count, Start: start, Data: fingerprint.AppendSymbolData(nil, m, count, start, syms),
	})}}
}

// servedSymbols returns the Symbols parts of m.
func servedSymbols(t *testing.T, m *pb.Message) []rangesync.Symbols {
	t.Helper()
	var out []rangesync.Symbols
	for _, p := range m.GetParts() {
		if p.GetSymbols() == nil {
			continue
		}
		sy, err := rangesync.ParseSymbols(p.GetSymbols())
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, sy)
	}
	return out
}

// symbolCount returns the number of coded symbols of sy.
func symbolCount(t *testing.T, sy rangesync.Symbols) uint64 {
	t.Helper()
	syms, err := fingerprint.ParseSymbolData(fingerprint.NewFloatMapper(), sy.Count, sy.Start, sy.Data)
	if err != nil {
		t.Fatal(err)
	}
	return uint64(len(syms))
}

// TestSessionSymbolLimits feeds sessions the symbol-stream abuses of
// proposal §6.4 and §9 (T2.5): More past cap(R), past the session budget,
// for a range that is not streaming, endless More, and lies in the peer's
// Symbols. Each one is a violation; the service resets the stream and
// scores the peer for it (TestReconcileAdversarial).
func TestSessionSymbolLimits(t *testing.T) {
	t.Parallel()

	root := rangeOf("01")
	st := status(root, 12)
	scope := mustScope(t, st, st)
	rng := newRand(25)
	small := randIDs(rng, root, 50)   // a scan-path root: cap(R) unlimited
	large := randIDs(rng, root, 4100) // past ScanLimit: cap(R) = RootSymbols

	responder := func(ids []fingerprint.ID) *rangesync.Session {
		l := newFakeLocal(ids...)
		l.status = &st
		return rangesync.NewResponder(rangesync.Config{Scope: scope, Local: l, Status: st, PeerStatus: st})
	}
	// serving returns a responder over ids that served the first batch of
	// root for a peer that holds one more element.
	serving := func(t *testing.T, ids []fingerprint.ID) *rangesync.Session {
		t.Helper()
		s := responder(ids)
		out, _, err := s.Handle(wire(t, msg(fpPart(root, sum(uint64(len(ids))+1, 1)))))
		if err != nil || len(servedSymbols(t, out)) != 1 {
			t.Fatalf("no symbols: %v %v", out, err)
		}
		return s
	}
	initiator := func(t *testing.T, cfg rangesync.Config, ids []fingerprint.ID, open ...fingerprint.Range) *rangesync.Session {
		t.Helper()
		l := newFakeLocal(ids...)
		l.status = &st
		cfg.Scope, cfg.Local, cfg.Status, cfg.PeerStatus, cfg.Open = scope, l, st, st, open
		s, _, err := rangesync.NewInitiator(cfg)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	kids := root.Children(fingerprint.FanoutBits)

	t.Run("more up to the capacity", func(t *testing.T) {
		t.Parallel()
		s := serving(t, large)
		out, _, err := s.Handle(wire(t, msg(limitMore(root, fingerprint.RootSymbols))))
		if err != nil {
			t.Fatal(err)
		}
		sys := servedSymbols(t, out)
		if len(sys) != 1 || !sys[0].Exhausted || sys[0].Start+symbolCount(t, sys[0]) != fingerprint.RootSymbols {
			t.Fatalf("got %+v, want the stream to its capacity, exhausted", sys)
		}
	})

	t.Run("budget across streams", func(t *testing.T) {
		t.Parallel()
		// two scan-path streams; the first takes 12000 symbols, the second
		// ends at the budget
		ids := slices.Concat(randIDs(newRand(1), kids[0], 20), randIDs(newRand(2), kids[1], 20))
		s := responder(ids)
		out, _, err := s.Handle(wire(t, msg(fpPart(kids[0], sum(21, 1)), fpPart(kids[1], sum(21, 2)))))
		if err != nil || len(servedSymbols(t, out)) != 2 {
			t.Fatalf("no symbols: %v %v", out, err)
		}
		sent := map[fingerprint.RangeKey]uint64{}
		for _, sy := range servedSymbols(t, out) {
			sent[sy.Range.Key()] = symbolCount(t, sy)
		}
		var total uint64
		for _, step := range []struct {
			r    fingerprint.Range
			upTo uint64
		}{{kids[0], 8000}, {kids[0], 12000}, {kids[1], 8000}} {
			out, _, err := s.Handle(wire(t, msg(limitMore(step.r, step.upTo))))
			if err != nil {
				t.Fatal(err)
			}
			for _, sy := range servedSymbols(t, out) {
				sent[sy.Range.Key()] = sy.Start + symbolCount(t, sy)
				if sy.Range.Equal(kids[1]) && !sy.Exhausted {
					t.Fatalf("batch to %d not exhausted at the budget", sent[sy.Range.Key()])
				}
			}
		}
		for _, n := range sent {
			total += n
		}
		if total != rangesync.SessionSymbolBudget {
			t.Fatalf("sent %d symbols, want the budget %d", total, rangesync.SessionSymbolBudget)
		}
		// the stream that reached the budget takes no More
		if _, _, err := s.Handle(wire(t, msg(limitMore(kids[1], 9000)))); !isInvalid(err) {
			t.Fatalf("more after the budget: %v, want invalid", err)
		}
	})

	t.Run("endless more", func(t *testing.T) {
		t.Parallel()
		s := serving(t, small)
		sent, rounds := uint64(rangesync.MinBatch), 0
		for exhausted := false; !exhausted; rounds++ {
			if rounds > 100 {
				t.Fatal("stream does not end")
			}
			out, _, err := s.Handle(wire(t, msg(limitMore(root, 2*sent))))
			if err != nil {
				t.Fatal(err)
			}
			sys := servedSymbols(t, out)
			if len(sys) != 1 {
				t.Fatalf("round %d: %d batches", rounds, len(sys))
			}
			sent, exhausted = sys[0].Start+symbolCount(t, sys[0]), sys[0].Exhausted
		}
		if sent != rangesync.SessionSymbolBudget {
			t.Fatalf("stream ended at %d, want the budget %d", sent, rangesync.SessionSymbolBudget)
		}
		if _, _, err := s.Handle(wire(t, msg(limitMore(root, 2*sent)))); !isInvalid(err) {
			t.Fatalf("more after exhausted: %v, want invalid", err)
		}
	})

	for _, tc := range []struct {
		name    string
		session func(t *testing.T) *rangesync.Session
		msgs    []*pb.Message
	}{
		{
			"more past the capacity", func(t *testing.T) *rangesync.Session { t.Helper(); return serving(t, large) },
			[]*pb.Message{msg(limitMore(root, fingerprint.RootSymbols+1))},
		},
		{
			"more past the capacity later", func(t *testing.T) *rangesync.Session { t.Helper(); return serving(t, large) },
			[]*pb.Message{msg(limitMore(root, 16)), msg(limitMore(root, 1<<40))},
		},
		{
			"more for an unknown range", func(t *testing.T) *rangesync.Session { t.Helper(); return serving(t, small) },
			[]*pb.Message{msg(limitMore(kids[3], 16))},
		},
		{
			"symbols with count 0", func(t *testing.T) *rangesync.Session { t.Helper(); return initiator(t, rangesync.Config{}, small) },
			[]*pb.Message{msg(limitBatch(1, root, 0, 0, 4))},
		},
		{"symbols with a count past the peer's", func(t *testing.T) *rangesync.Session {
			t.Helper()
			return initiator(t, rangesync.Config{PeerCount: 100}, small)
		}, []*pb.Message{msg(limitBatch(1, root, 100+rangesync.ItemsPageSize+1, 0, 4))}},
		{
			"symbols that change the count", func(t *testing.T) *rangesync.Session { t.Helper(); return initiator(t, rangesync.Config{}, small) },
			[]*pb.Message{msg(limitBatch(1, root, 53, 0, 4)), msg(limitBatch(1, root, 54, 4, 12))},
		},
		{
			"first batch past the capacity", func(t *testing.T) *rangesync.Session { t.Helper(); return initiator(t, rangesync.Config{}, large) },
			[]*pb.Message{msg(limitBatch(1, root, 4101, 0, fingerprint.RootSymbols+1))},
		},
		{"symbols for a range without capacity", func(t *testing.T) *rangesync.Session {
			t.Helper()
			// 12 bits below the root, past LeafBits 8: no cache, and the
			// claimed count is past ScanLimit
			st8 := status(root, 8)
			l := newFakeLocal(large...)
			s, _, err := rangesync.NewInitiator(rangesync.Config{Scope: mustScope(t, st8, st8), Local: l, Status: st8, PeerStatus: st8, Open: []fingerprint.Range{rangeOf("01000000000000")}})
			if err != nil {
				t.Fatal(err)
			}
			return s
		}, []*pb.Message{msg(limitBatch(1, rangeOf("01000000000000"), fingerprint.ScanLimit+1, 0, 4))}},
		{"symbols past the session budget", func(t *testing.T) *rangesync.Session {
			t.Helper()
			return initiator(t, rangesync.Config{}, small, kids[0], kids[1], kids[2])
		}, []*pb.Message{
			msg(limitBatch(1, kids[0], 5, 0, rangesync.MaxBatchSymbols)),
			msg(limitBatch(2, kids[1], 5, 0, rangesync.MaxBatchSymbols)),
			msg(limitBatch(3, kids[2], 5, 0, 1)),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := tc.session(t)
			var err error
			for _, m := range tc.msgs {
				if _, _, err = s.Handle(wire(t, m)); err != nil {
					break
				}
			}
			if !isInvalid(err) {
				t.Fatalf("got %v, want invalid", err)
			}
		})
	}
}

// TestSessionMaxScansPerMessage opens a session with more scan-path ranges
// than MaxScansPerMessage: the responder serves at most MaxScansPerMessage
// of their first batches per Message and defers the others, without a
// violation.
func TestSessionMaxScansPerMessage(t *testing.T) {
	t.Parallel()

	root := rangeOf("01")
	st := status(root, 4) // ranges more than 4 bits below the root have no cache
	scope := mustScope(t, st, st)
	rs := root.Children(7) // 128 ranges 7 bits below the root
	rng := newRand(26)
	ids := make([]fingerprint.ID, 0, 2*len(rs))
	parts := make([]*pb.Part, len(rs))
	for i, r := range rs {
		ids = append(ids, randIDs(rng, r, 2)...)
		parts[i] = fpPart(r, sum(3, byte(i)))
	}
	l := newFakeLocal(ids...)
	l.status = &st
	s := rangesync.NewResponder(rangesync.Config{Scope: scope, Local: l, Status: st, PeerStatus: st})
	served := map[fingerprint.RangeKey]bool{}
	in := msg(parts...)
	for i := 0; len(served) < len(rs); i++ {
		if i > 2 {
			t.Fatalf("%d of %d ranges served after %d messages", len(served), len(rs), i)
		}
		out, _, err := s.Handle(wire(t, in))
		if err != nil {
			t.Fatal(err)
		}
		sys := servedSymbols(t, out)
		if len(sys) > rangesync.MaxScansPerMessage {
			t.Fatalf("message %d: %d scan-path batches, want at most %d", i, len(sys), rangesync.MaxScansPerMessage)
		}
		for _, sy := range sys {
			served[sy.Range.Key()] = true
		}
		in = msg()
	}
}

// TestSessionMappingGrinding is the mapping grinding of the T2.0 follow-up:
// ids x and y map to the same indices among the first BucketSymbols
// symbols, and x is held only by A, y only by B, in one bucket whose cached
// prefix is all both sides can stream. That prefix cannot peel them; the
// stream is exhausted, the bucket split and its children (scan path, where
// the mappings of x and y diverge) decode them.
func TestSessionMappingGrinding(t *testing.T) {
	t.Parallel()

	root := rangeOf("")
	bucket := rangeOf("01101100") // 8 bits below the root: BucketSymbols cached
	st := status(root, 12)
	m := fingerprint.NewFloatMapper()
	signature := func(id fingerprint.ID) string {
		syms := make([]fingerprint.CodedSymbol, fingerprint.BucketSymbols)
		fingerprint.Encode(m, syms, 0, id, 1)
		var b []byte
		for i, s := range syms {
			if s.Count != 0 {
				b = fmt.Appendf(b, "%d,", i)
			}
		}
		return string(b)
	}
	rng := newRand(27)
	var x, y fingerprint.ID
	seen := map[string]fingerprint.ID{}
	for found := false; !found; {
		id := randID(rng, bucket)
		sig := signature(id)
		if other, ok := seen[sig]; ok && other != id {
			x, y, found = other, id, true
		}
		seen[sig] = id
		if len(seen) > 1<<16 {
			t.Fatal("no ground pair")
		}
	}
	shared := randIDs(rng, bucket, fingerprint.ScanLimit+100)
	la := newFakeLocal(append(slices.Clone(shared), x)...)
	lb := newFakeLocal(append(slices.Clone(shared), y)...)
	a, b, first := symbolsPair(t, st, la, lb, bucket)
	var kinds partKinds
	wantsA, wantsB, _ := drive(t, a, first, b, kinds.inspect)
	assertSet(t, wantsA, []fingerprint.ID{y})
	assertSet(t, wantsB, []fingerprint.ID{x})
	if a.Stats().Exhausted == 0 || kinds.count("A:fp") < 1+1<<fingerprint.FanoutBits {
		t.Fatalf("stats %+v, messages %v: want the bucket exhausted and split", a.Stats(), kinds.msgs)
	}
}
