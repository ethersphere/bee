// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
)

func sum(count uint64, d byte) fingerprint.Summary {
	if count == 0 {
		return fingerprint.Summary{}
	}
	return fingerprint.Summary{Count: count, Digest: [32]byte{d}}
}

func TestDecide(t *testing.T) {
	t.Parallel()

	scope := rangesync.Scope{Root: rangeOf("01"), LeafDepth: 14}
	// a root 3 bits deep: splits land on 7 and 11, and 11+4 > 14
	misaligned := rangesync.Scope{Root: rangeOf("011"), LeafDepth: 14}
	bits := func(n int) fingerprint.Range { return rangeOf("01101100110011001100"[:n]) }

	for _, tc := range []struct {
		name          string
		scope         rangesync.Scope
		r             fingerprint.Range
		local, remote fingerprint.Summary
		initiator     bool
		want          rangesync.Decision
	}{
		{"equal", scope, bits(2), sum(5, 1), sum(5, 1), false, rangesync.DecisionEqual},
		{"both empty", scope, bits(2), sum(0, 0), sum(0, 0), false, rangesync.DecisionEqual},
		{"peer empty", scope, bits(2), sum(5, 1), sum(0, 0), false, rangesync.DecisionSendItems},
		{"local empty", scope, bits(2), sum(0, 0), sum(5, 1), false, rangesync.DecisionRequestItems},
		{"peer empty at leaf depth", scope, bits(14), sum(5, 1), sum(0, 0), false, rangesync.DecisionSendItems},
		{"local empty at leaf depth", scope, bits(14), sum(0, 0), sum(5, 1), false, rangesync.DecisionRequestItems},
		{"differ", scope, bits(2), sum(5, 1), sum(6, 2), false, rangesync.DecisionSplit},
		{"equal counts hide a difference", scope, bits(2), sum(5, 1), sum(5, 2), false, rangesync.DecisionSplit},
		{"last splittable depth", scope, bits(10), sum(5, 1), sum(6, 2), false, rangesync.DecisionSplit},
		{"one past the last splittable depth", scope, bits(11), sum(5, 1), sum(6, 2), false, rangesync.DecisionExchangeItems},
		{"at leaf depth", scope, bits(14), sum(5, 1), sum(6, 2), true, rangesync.DecisionExchangeItems},
		{"below leaf depth", scope, bits(17), sum(5, 1), sum(6, 2), false, rangesync.DecisionExchangeItems},
		{"large difference, initiator", scope, bits(2), sum(rangesync.LargeDiff+2, 1), sum(1, 2), true, rangesync.DecisionStripe},
		{"large difference, other sign", scope, bits(2), sum(1, 1), sum(rangesync.LargeDiff+2, 2), true, rangesync.DecisionStripe},
		{"large difference, responder", scope, bits(2), sum(rangesync.LargeDiff+2, 1), sum(1, 2), false, rangesync.DecisionSplit},
		{"difference of exactly LargeDiff", scope, bits(2), sum(rangesync.LargeDiff+1, 1), sum(1, 2), true, rangesync.DecisionSplit},
		{"large difference not splittable", scope, bits(11), sum(rangesync.LargeDiff+2, 1), sum(1, 2), true, rangesync.DecisionExchangeItems},
		{"misaligned root", misaligned, bits(3), sum(5, 1), sum(6, 2), false, rangesync.DecisionSplit},
		{"misaligned level 1", misaligned, bits(7), sum(5, 1), sum(6, 2), false, rangesync.DecisionSplit},
		{"misaligned level 2", misaligned, bits(11), sum(5, 1), sum(6, 2), false, rangesync.DecisionExchangeItems},
		{"root at leaf depth", rangesync.Scope{Root: rangeOf("01"), LeafDepth: 2}, bits(2), sum(5, 1), sum(6, 2), false, rangesync.DecisionExchangeItems},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := rangesync.Decide(rangesync.DecisionInput{Scope: tc.scope, Range: tc.r, Local: tc.local, Remote: tc.remote, Initiator: tc.initiator})
			if got != tc.want {
				t.Fatalf("got decision %d, want %d", got, tc.want)
			}
		})
	}
}

func TestNewInitiatorOpen(t *testing.T) {
	t.Parallel()

	scope := rangesync.Scope{Root: rangeOf("01"), LeafDepth: 14}
	local := newFakeLocal(randIDs(newRand(3), scope.Root, 100)...)
	depth := func(bits int) []fingerprint.Range { return rangeOf("01").Children(uint8(bits - 2)) }

	for _, tc := range []struct {
		name  string
		open  []fingerprint.Range
		valid bool
	}{
		{"default", nil, true},
		{"root", []fingerprint.Range{rangeOf("01")}, true},
		{"stripes", []fingerprint.Range{rangeOf("0100"), rangeOf("0110")}, true},
		{"halves", []fingerprint.Range{rangeOf("010"), rangeOf("011")}, true},
		{"max parts", depth(12), true},
		{"over max parts", depth(13)[:rangesync.MaxParts+1], false},
		{"overlapping", []fingerprint.Range{rangeOf("0110"), rangeOf("01")}, false},
		{"duplicate", []fingerprint.Range{rangeOf("0110"), rangeOf("0110")}, false},
		{"out of scope", []fingerprint.Range{rangeOf("10")}, false},
		{"too deep", []fingerprint.Range{rangeOf("0110110011001100001")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, open, err := rangesync.NewInitiator(rangesync.Config{Scope: scope, Local: local, Open: tc.open})
			if !tc.valid {
				if !errors.Is(err, rangesync.ErrInvalidOpening) || isInvalid(err) {
					t.Fatalf("got %v, want %v", err, rangesync.ErrInvalidOpening)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := tc.open
			if want == nil {
				want = []fingerprint.Range{scope.Root}
			}
			if len(open.Parts) != len(want) {
				t.Fatalf("got %d parts, want %d", len(open.Parts), len(want))
			}
			for i, p := range wire(t, open).Parts {
				r, s, err := rangesync.ParseFingerprint(p.GetFP())
				if err != nil {
					t.Fatal(err)
				}
				ls, _ := local.Summary(want[i])
				if !r.Equal(want[i]) || s != ls {
					t.Fatalf("part %d: got %v %+v, want %v %+v", i, r, s, want[i], ls)
				}
			}
		})
	}

	t.Run("local error", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("boom")
		_, _, err := rangesync.NewInitiator(rangesync.Config{Scope: scope, Local: &fakeLocal{err: boom}})
		if !errors.Is(err, boom) || isInvalid(err) {
			t.Fatalf("got %v, want %v", err, boom)
		}
	})
}

// TestSessionTermination checks the end of a session: equal sets cost one
// round trip, and a session that ended cannot be used.
func TestSessionTermination(t *testing.T) {
	t.Parallel()

	scope := rangesync.Scope{Root: rangeOf(""), LeafDepth: 12}
	ids := randIDs(newRand(4), scope.Root, 500)
	a, open, err := rangesync.NewInitiator(rangesync.Config{Scope: scope, Local: newFakeLocal(ids...)})
	if err != nil {
		t.Fatal(err)
	}
	b := rangesync.NewResponder(rangesync.Config{Scope: scope, Local: newFakeLocal(ids...)})

	out, wants, err := b.Handle(wire(t, open))
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || len(out.Parts) != 0 || len(wants) != 0 {
		t.Fatalf("responder: got %v, %d wants, want the empty message", out, len(wants))
	}
	out, wants, err = a.Handle(wire(t, out))
	if err != nil {
		t.Fatal(err)
	}
	if out != nil || len(wants) != 0 {
		t.Fatalf("initiator: got %v, %d wants, want the end of the session", out, len(wants))
	}
	if _, _, err := a.Handle(msg()); err == nil {
		t.Fatal("session usable after its end")
	}
}

// TestSessionItems covers the item exchanges: listing everything to an empty
// peer, requesting everything when empty, and the Reply exchange at the leaf
// depth, with paging over several messages.
func TestSessionItems(t *testing.T) {
	t.Parallel()

	rng := newRand(5)
	root := rangeOf("01")

	t.Run("send items", func(t *testing.T) {
		t.Parallel()
		scope := rangesync.Scope{Root: root, LeafDepth: 14}
		ids := randIDs(rng, root, 3*rangesync.ItemsPageSize+5)
		a, open, err := rangesync.NewInitiator(rangesync.Config{Scope: scope, Local: newFakeLocal()})
		if err != nil {
			t.Fatal(err)
		}
		b := rangesync.NewResponder(rangesync.Config{Scope: scope, Local: newFakeLocal(ids...)})
		var pages int
		checkPages := pageChecker(t, &pages)
		wantsA, wantsB, _ := drive(t, a, open, b, checkPages)
		assertSet(t, wantsA, ids)
		if len(wantsB) != 0 {
			t.Fatalf("responder wants %d", len(wantsB))
		}
		if pages < 4 {
			t.Fatalf("got %d pages, want at least 4", pages)
		}
	})

	t.Run("request items", func(t *testing.T) {
		t.Parallel()
		scope := rangesync.Scope{Root: root, LeafDepth: 14}
		ids := randIDs(rng, root, 2*rangesync.ItemsPageSize)
		a, open, err := rangesync.NewInitiator(rangesync.Config{Scope: scope, Local: newFakeLocal(ids...)})
		if err != nil {
			t.Fatal(err)
		}
		b := rangesync.NewResponder(rangesync.Config{Scope: scope, Local: newFakeLocal()})
		var request bool
		inspect := func(fromInitiator bool, m *pb.Message) {
			if fromInitiator {
				return
			}
			_, items := partsOf(t, m)
			for _, it := range items {
				if it.Reply && len(it.IDs) == 0 && !it.More && it.Range.Equal(root) {
					request = true
				}
			}
		}
		wantsA, wantsB, messages := drive(t, a, open, b, inspect, pageChecker(t, new(int)))
		assertSet(t, wantsB, ids)
		if len(wantsA) != 0 || !request || messages != 4 {
			t.Fatalf("initiator wants %d, request %v, %d messages", len(wantsA), request, messages)
		}
	})

	t.Run("exchange items", func(t *testing.T) {
		t.Parallel()
		// the root is at the leaf depth, so it is reconciled by items
		scope := rangesync.Scope{Root: root, LeafDepth: 2}
		common := randIDs(rng, root, 3000)
		onlyA := randIDs(rng, root, 1500)
		onlyB := randIDs(rng, root, 20000) // more than one message of pages
		la := newFakeLocal(append(slices.Clone(common), onlyA...)...)
		lb := newFakeLocal(append(slices.Clone(common), onlyB...)...)
		a, open, err := rangesync.NewInitiator(rangesync.Config{Scope: scope, Local: la})
		if err != nil {
			t.Fatal(err)
		}
		b := rangesync.NewResponder(rangesync.Config{Scope: scope, Local: lb})

		// the initiator's answers never repeat an ID the responder listed
		listed := map[fingerprint.ID]bool{}
		inspect := func(fromInitiator bool, m *pb.Message) {
			_, items := partsOf(t, m)
			for _, it := range items {
				for _, id := range it.IDs {
					if !fromInitiator {
						listed[id] = true
					} else if listed[id] {
						t.Fatalf("answer repeats listed id %x", id[:4])
					}
				}
			}
		}
		wantsA, wantsB, messages := drive(t, a, open, b, inspect, pageChecker(t, new(int)))
		assertSet(t, wantsA, onlyB)
		assertSet(t, wantsB, onlyA)
		if messages < 4 {
			t.Fatalf("got %d messages, want the pages over several", messages)
		}
	})
}

// TestSessionAnswerEnd checks that an answer stream that started ends with
// an empty last page when the peer's last Reply page covers none of this
// node's IDs.
func TestSessionAnswerEnd(t *testing.T) {
	t.Parallel()

	root := rangeOf("01")
	scope := rangesync.Scope{Root: root, LeafDepth: 2}
	ids := randIDs(newRand(12), root, 4)
	slices.SortFunc(ids, fingerprint.ID.Compare)
	mine, peer1, peer2 := ids[0], ids[1], ids[3]
	a, _, err := rangesync.NewInitiator(rangesync.Config{Scope: scope, Local: newFakeLocal(mine)})
	if err != nil {
		t.Fatal(err)
	}

	out, wants, err := a.Handle(msg(itemsPart(rangesync.Items{Range: root, IDs: []fingerprint.ID{peer1}, More: true, Reply: true})))
	if err != nil {
		t.Fatal(err)
	}
	_, its := partsOf(t, out)
	if len(wants) != 1 || len(its) != 1 || !slices.Equal(its[0].IDs, []fingerprint.ID{mine}) || !its[0].More {
		t.Fatalf("first answer %+v, %d wants", its, len(wants))
	}

	out, _, err = a.Handle(msg(itemsPart(rangesync.Items{Range: root, IDs: []fingerprint.ID{peer2}, After: &peer1, Reply: true})))
	if err != nil {
		t.Fatal(err)
	}
	_, its = partsOf(t, out)
	if len(its) != 1 || len(its[0].IDs) != 0 || its[0].More || its[0].After == nil || *its[0].After != mine {
		t.Fatalf("last answer %+v, want an empty last page after %x", its, mine[:4])
	}
}

// pageChecker returns an inspect function that checks the Items streams of
// each side: pages in ascending order, After continuity, page size, and More
// only on pages with IDs. It counts the pages in *pages.
func pageChecker(t *testing.T, pages *int) func(bool, *pb.Message) {
	t.Helper()
	type key struct {
		from bool
		r    fingerprint.RangeKey
	}
	last := map[key]*fingerprint.ID{}
	open := map[key]bool{}
	return func(from bool, m *pb.Message) {
		_, items := partsOf(t, m)
		for _, it := range items {
			*pages++
			k := key{from, it.Range.Key()}
			if open[k] {
				if it.After == nil || last[k] == nil || *it.After != *last[k] {
					t.Fatal("page does not continue its stream")
				}
			} else if it.After != nil {
				t.Fatal("first page has After")
			}
			if len(it.IDs) > rangesync.ItemsPageSize {
				t.Fatalf("page of %d IDs", len(it.IDs))
			}
			if len(it.IDs) > 0 {
				last[k] = idPtr(it.IDs[len(it.IDs)-1])
			}
			open[k] = it.More
		}
	}
}

// assertSet checks that got holds exactly the IDs of want, each once.
func assertSet(t *testing.T, got, want []fingerprint.ID) {
	t.Helper()
	g := slices.Clone(got)
	slices.SortFunc(g, fingerprint.ID.Compare)
	if len(slices.Compact(slices.Clone(g))) != len(g) {
		t.Fatal("duplicate IDs")
	}
	w := slices.Clone(want)
	slices.SortFunc(w, fingerprint.ID.Compare)
	w = slices.Compact(w)
	if !slices.Equal(g, w) {
		t.Fatalf("got %d IDs, want %d", len(g), len(w))
	}
}

// assertDifference checks that initiator a and responder b both report the
// session's full difference n_Δ, want: the IDs only one of them holds.
func assertDifference(t *testing.T, a, b *rangesync.Session, want int) {
	t.Helper()
	if da, db := a.Difference(), b.Difference(); da != uint64(want) || db != uint64(want) {
		t.Fatalf("difference: initiator %d, responder %d, want %d", da, db, want)
	}
}

type striperFunc func(r fingerprint.Range, local, remote uint64) bool

func (f striperFunc) Stripe(r fingerprint.Range, local, remote uint64) bool {
	return f(r, local, remote)
}

func TestSessionStriper(t *testing.T) {
	t.Parallel()

	scope := rangesync.Scope{Root: rangeOf("01"), LeafDepth: 14}
	big := &fakeLocal{summary: func(fingerprint.Range) (fingerprint.Summary, error) {
		return sum(rangesync.LargeDiff+10, 1), nil
	}}
	small := &fakeLocal{summary: func(fingerprint.Range) (fingerprint.Summary, error) { return sum(1, 2), nil }}
	never := striperFunc(func(fingerprint.Range, uint64, uint64) bool {
		t.Error("responder offered a range to its striper")
		return false
	})

	for _, take := range []bool{true, false} {
		var offered []fingerprint.Range
		striper := striperFunc(func(r fingerprint.Range, local, remote uint64) bool {
			if local != rangesync.LargeDiff+10 || remote != 1 {
				t.Errorf("offered counts %d, %d", local, remote)
			}
			offered = append(offered, r)
			return take
		})
		a, open, err := rangesync.NewInitiator(rangesync.Config{Scope: scope, Local: big, Striper: striper})
		if err != nil {
			t.Fatal(err)
		}
		b := rangesync.NewResponder(rangesync.Config{Scope: scope, Local: small, Striper: never})

		// the responder splits the root
		split, _, err := b.Handle(wire(t, open))
		if err != nil {
			t.Fatal(err)
		}
		fps, _ := partsOf(t, split)
		if len(fps) != 16 {
			t.Fatalf("responder sent %d fingerprints, want the split", len(fps))
		}
		out, _, err := a.Handle(wire(t, split))
		if err != nil {
			t.Fatal(err)
		}
		if len(offered) != 16 {
			t.Fatalf("offered %d ranges, want 16", len(offered))
		}
		fps, items := partsOf(t, out)
		if take && (len(fps) != 0 || len(items) != 0) {
			t.Fatalf("taken ranges: got %d fingerprints and %d items, want none", len(fps), len(items))
		}
		if !take && len(fps) != 256 {
			t.Fatalf("declined ranges: got %d fingerprints, want 256", len(fps))
		}
	}
}

// TestSessionValidation feeds a session messages that break the rules of
// Session.Handle; all but the last message of each case are valid.
func TestSessionValidation(t *testing.T) {
	t.Parallel()

	rng := newRand(6)
	root := rangeOf("01")
	scope := rangesync.Scope{Root: root, LeafDepth: 14}
	ids := randIDs(rng, root, 50)
	slices.SortFunc(ids, fingerprint.ID.Compare)
	x, y, z := ids[0], ids[1], ids[2]
	local := newFakeLocal(ids[10:]...)
	children := func(r fingerprint.Range) []*pb.Part {
		parts := make([]*pb.Part, 0, 1<<fingerprint.FanoutBits)
		for _, c := range r.Children(fingerprint.FanoutBits) {
			parts = append(parts, fpPart(c, sum(1, 9)))
		}
		return parts
	}
	initiator := func(scope rangesync.Scope) func(t *testing.T) *rangesync.Session {
		return func(t *testing.T) *rangesync.Session {
			t.Helper()
			s, _, err := rangesync.NewInitiator(rangesync.Config{Scope: scope, Local: local})
			if err != nil {
				t.Fatal(err)
			}
			return s
		}
	}
	responder := func(scope rangesync.Scope) func(t *testing.T) *rangesync.Session {
		return func(*testing.T) *rangesync.Session {
			return rangesync.NewResponder(rangesync.Config{Scope: scope, Local: local})
		}
	}
	items := func(r fingerprint.Range, after *fingerprint.ID, more, reply bool, ids ...fingerprint.ID) *pb.Part {
		return itemsPart(rangesync.Items{Range: r, IDs: ids, After: after, More: more, Reply: reply})
	}
	opening := msg(fpPart(root, sum(3, 3)))
	busy := &pb.Message{Busy: true}
	// a page per child of the root, 16 × ItemsPageSize IDs: over MaxMessageBytes
	oversize := make([]*pb.Part, 0, 16)
	for _, r := range root.Children(4) {
		page := randIDs(rng, r, rangesync.ItemsPageSize)
		slices.SortFunc(page, fingerprint.ID.Compare)
		oversize = append(oversize, itemsPart(rangesync.Items{Range: r, IDs: page}))
	}

	for _, tc := range []struct {
		name     string
		session  func(t *testing.T) *rangesync.Session
		messages []*pb.Message
	}{
		{"split of a range above the depth bound", initiator(rangesync.Scope{Root: root, LeafDepth: 5}), []*pb.Message{msg(children(root)...)}},
		{"split at the leaf depth", initiator(rangesync.Scope{Root: root, LeafDepth: 2}), []*pb.Message{msg(children(root)...)}},
		{"incomplete split", initiator(scope), []*pb.Message{msg(children(root)[1:]...)}},
		{"duplicate child", initiator(scope), []*pb.Message{msg(append(children(root)[1:], children(root)[1])...)}},
		{"split of an unsent range", initiator(scope), []*pb.Message{msg(children(rangeOf("0110"))...)}},
		{"range answered twice", initiator(scope), []*pb.Message{msg(children(root)...), msg(children(root)...)}},
		{"items and a split for one range", initiator(scope), []*pb.Message{msg(append([]*pb.Part{items(root, nil, false, false)}, children(root)...)...)}},
		{"items for an unsolicited range", initiator(scope), []*pb.Message{msg(items(rangeOf("0110"), nil, false, false))}},
		{"continuation without a stream", initiator(scope), []*pb.Message{msg(items(root, &x, false, false, y))}},
		{"broken cursor", initiator(scope), []*pb.Message{msg(items(root, nil, true, false, x, y)), msg(items(root, &x, false, false, z))}},
		{"continuation of an ended stream", initiator(scope), []*pb.Message{msg(items(root, nil, false, false, x)), msg(items(root, &x, false, false, y))}},
		{"second stream for a range", initiator(scope), []*pb.Message{msg(items(root, nil, false, false)), msg(items(root, nil, false, false))}},
		{"reply flag changes", initiator(scope), []*pb.Message{msg(items(root, nil, true, false, x)), msg(items(root, &x, false, true, y))}},
		{"empty message with an open stream", initiator(scope), []*pb.Message{msg(items(root, nil, true, false, x)), msg()}},
		{"answer to a reply never sent", responder(scope), []*pb.Message{opening, msg(items(root, nil, false, false, x))}},
		{"busy with parts", initiator(scope), []*pb.Message{{Busy: true, Parts: children(root)}}},
		{"busy to a responder", responder(scope), []*pb.Message{busy}},
		{"busy after the first reply", initiator(scope), []*pb.Message{msg(items(root, nil, true, false, x)), busy}},
		{"empty opening", responder(scope), []*pb.Message{msg()}},
		{"opening with items", responder(scope), []*pb.Message{msg(items(root, nil, false, true))}},
		{"overlapping opening", responder(scope), []*pb.Message{msg(fpPart(root, sum(1, 1)), fpPart(rangeOf("0110"), sum(1, 1)))}},
		{"symbols", initiator(scope), []*pb.Message{msg(&pb.Part{Kind: &pb.Part_Symbols{Symbols: &pb.Symbols{Range: rangesync.EncodeRange(root)}}})}},
		{"more", responder(scope), []*pb.Message{msg(&pb.Part{Kind: &pb.Part_More{More: &pb.More{Range: rangesync.EncodeRange(root)}}})}},
		{"fingerprint out of scope", responder(scope), []*pb.Message{msg(fpPart(rangeOf("10"), sum(1, 1)))}},
		{"nil message", responder(scope), []*pb.Message{nil}},
		{"over max bytes", initiator(scope), []*pb.Message{msg(oversize...)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := tc.session(t)
			for i, m := range tc.messages {
				_, _, err := s.Handle(m)
				if i < len(tc.messages)-1 {
					if err != nil {
						t.Fatalf("message %d: %v", i, err)
					}
					continue
				}
				if !isInvalid(err) {
					t.Fatalf("got %v, want invalid", err)
				}
			}
			if _, _, err := s.Handle(msg()); err == nil {
				t.Fatal("session usable after a violation")
			}
		})
	}

	t.Run("busy", func(t *testing.T) {
		t.Parallel()
		_, _, err := initiator(scope)(t).Handle(busy)
		if !errors.Is(err, rangesync.ErrPeerBusy) || isInvalid(err) {
			t.Fatalf("got %v, want %v", err, rangesync.ErrPeerBusy)
		}
	})

	t.Run("audit round ignored", func(t *testing.T) {
		t.Parallel()
		m := msg(fpPart(root, sum(3, 3)))
		m.AuditRound = 5
		if _, _, err := responder(scope)(t).Handle(m); err != nil {
			t.Fatal(err)
		}
	})

	// The peer ends its answers to our Reply stream before our last page.
	t.Run("answer stream ends early", func(t *testing.T) {
		t.Parallel()
		leaf := rangesync.Scope{Root: root, LeafDepth: 2}
		b := rangesync.NewResponder(rangesync.Config{Scope: leaf, Local: newFakeLocal(randIDs(rng, root, 20000)...)})
		out, _, err := b.Handle(opening)
		if err != nil {
			t.Fatal(err)
		}
		_, its := partsOf(t, out)
		if len(its) == 0 || !its[len(its)-1].More || !its[0].Reply {
			t.Fatal("want an unfinished Reply stream")
		}
		_, _, err = b.Handle(msg(items(root, nil, false, false)))
		if !isInvalid(err) {
			t.Fatalf("got %v, want invalid", err)
		}
	})
}

func TestSessionLocalError(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	scope := rangesync.Scope{Root: rangeOf("01"), LeafDepth: 14}
	b := rangesync.NewResponder(rangesync.Config{Scope: scope, Local: &fakeLocal{err: boom}})
	_, _, err := b.Handle(msg(fpPart(scope.Root, sum(3, 3))))
	if !errors.Is(err, boom) || isInvalid(err) {
		t.Fatalf("got %v, want %v not wrapping ErrInvalidMessage", err, boom)
	}
	if _, _, err := b.Handle(msg()); err == nil {
		t.Fatal("session usable after a local error")
	}
}

// TestSessionLimits checks that every message stays within MaxParts and
// MaxMessageBytes with splits kept whole, and that the state kept for the
// peer's Reply pages stays bounded when their answers do not fit.
func TestSessionLimits(t *testing.T) {
	t.Parallel()

	t.Run("spread differences", func(t *testing.T) {
		t.Parallel()
		rng := newRand(7)
		scope := rangesync.Scope{Root: rangeOf(""), LeafDepth: 12}
		common := randIDs(rng, scope.Root, 2000)
		la := newFakeLocal(append(slices.Clone(common), randIDs(rng, scope.Root, 3000)...)...)
		lb := newFakeLocal(append(slices.Clone(common), randIDs(rng, scope.Root, 3000)...)...)
		a, open, err := rangesync.NewInitiator(rangesync.Config{Scope: scope, Local: la})
		if err != nil {
			t.Fatal(err)
		}
		b := rangesync.NewResponder(rangesync.Config{Scope: scope, Local: lb})
		var full bool
		first := true
		inspect := func(_ bool, m *pb.Message) {
			fps, _ := partsOf(t, m)
			if len(m.Parts) > rangesync.MaxParts-1<<fingerprint.FanoutBits {
				full = true
			}
			if first {
				first = false
				return
			}
			// splits are whole: every parent has all its children
			children := map[fingerprint.RangeKey]int{}
			for _, r := range fps {
				children[fingerprint.NewRange(r.Prefix, r.Bits-fingerprint.FanoutBits).Key()]++
			}
			for _, n := range children {
				if n != 1<<fingerprint.FanoutBits {
					t.Fatalf("split with %d children", n)
				}
			}
		}
		drive(t, a, open, b, inspect)
		if !full {
			t.Fatal("no message reached MaxParts; the test does not exercise the limit")
		}
	})

	// An adversarial peer answers 1024 opening ranges with Reply pages that
	// each cover all of the node's IDs in the range but list only one of
	// them, so the answers are far larger than a message.
	for _, pagesPerRange := range []int{1, 2} {
		t.Run("pending replies", func(t *testing.T) {
			t.Parallel()
			rng := newRand(8)
			scope := rangesync.Scope{Root: rangeOf(""), LeafDepth: 12}
			ranges := scope.Root.Children(10)
			all := make([]fingerprint.ID, 0, 32*len(ranges))
			for _, r := range ranges {
				all = append(all, randIDs(rng, r, 32)...)
			}
			local := newFakeLocal(all...)
			a, _, err := rangesync.NewInitiator(rangesync.Config{Scope: scope, Local: local, Open: ranges})
			if err != nil {
				t.Fatal(err)
			}

			answered := map[fingerprint.ID]bool{}
			listed := map[fingerprint.ID]bool{}
			answerMessages := 0
			check := func(out *pb.Message) {
				t.Helper()
				answerMessages++
				_, its := partsOf(t, wire(t, out))
				for _, it := range its {
					if it.Reply {
						t.Fatal("answer with Reply")
					}
					for _, id := range it.IDs {
						if answered[id] {
							t.Fatalf("id %x answered twice", id[:4])
						}
						answered[id] = true
					}
				}
				withExcl, excluded, intervals := a.PendingReplies()
				if withExcl > rangesync.MaxPendingReplies || excluded > rangesync.MaxPendingReplies*rangesync.ItemsPageSize || intervals > len(ranges)+2*rangesync.MaxPendingReplies {
					t.Fatalf("pending: %d with exclusion, %d excluded, %d intervals", withExcl, excluded, intervals)
				}
			}

			for page := 0; page < pagesPerRange; page++ {
				parts := make([]*pb.Part, 0, len(ranges))
				for _, r := range ranges {
					mine := local.in(r)
					// page k lists one ID and covers up to the end of its share
					step := len(mine) / pagesPerRange
					var after *fingerprint.ID
					if page > 0 {
						after = idPtr(mine[page*step-1])
					}
					last := mine[(page+1)*step-1]
					listed[last] = true
					parts = append(parts, itemsPart(rangesync.Items{Range: r, IDs: []fingerprint.ID{last}, After: after, More: page < pagesPerRange-1, Reply: true}))
				}
				out, wants, err := a.Handle(wire(t, msg(parts...)))
				if err != nil {
					t.Fatal(err)
				}
				if len(wants) != 0 {
					t.Fatalf("%d wants for held IDs", len(wants))
				}
				check(out)
			}
			for i := 0; ; i++ {
				if i > 1000 {
					t.Fatal("answers do not end")
				}
				out, _, err := a.Handle(msg())
				if err != nil {
					t.Fatal(err)
				}
				if out == nil {
					break
				}
				check(out)
			}
			for _, id := range all {
				if !answered[id] && !listed[id] {
					t.Fatalf("id %x never answered", id[:4])
				}
			}
			// both paths ran: answers that leave the listed ID out, and
			// degraded answers that send it back
			var excluded, degraded int
			for id := range listed {
				if answered[id] {
					degraded++
				} else {
					excluded++
				}
			}
			if excluded == 0 || degraded == 0 || answerMessages < 2 {
				t.Fatalf("%d excluded, %d degraded, %d answer messages", excluded, degraded, answerMessages)
			}
		})
	}
}

// TestSessionConverges drives two sessions over in-memory sets against each
// other and checks that each side's wants are exactly what the other holds
// and it lacks, inside the opened ranges. Each case runs in the Phase-1 form
// (no Statuses: splits and item exchanges) and with symbol streams, whose
// windows end at the caches of the Index for ranges over ScanLimit.
func TestSessionConverges(t *testing.T) {
	t.Parallel()

	whole := status(rangeOf(""), fingerprint.LeafBits)
	for _, tc := range []struct {
		name                 string
		local, remote        rangesync.Status
		common, onlyA, onlyB int
		// paired puts each A-only ID in the leaf range of a B-only one, so
		// counts are equal at every level
		paired bool
		// open is the opening (default: the session root) and gen the range
		// the IDs are drawn from (default: the local root)
		open []fingerprint.Range
		gen  *fingerprint.Range
		// longStreams requires an Items stream spanning three messages
		longStreams bool
	}{
		{name: "equal", local: whole, remote: whole, common: 3000},
		{name: "one", local: whole, remote: whole, common: 3000, onlyA: 1},
		{name: "twenty", local: whole, remote: whole, common: 3000, onlyA: 10, onlyB: 10},
		{name: "thousand", local: whole, remote: whole, common: 3000, onlyA: 500, onlyB: 500},
		{name: "responder empty", local: whole, remote: whole, onlyA: 3000},
		{name: "initiator empty", local: whole, remote: whole, onlyB: 3000},
		{name: "disjoint", local: whole, remote: whole, onlyA: 2000, onlyB: 2000},
		{name: "equal counts at every level", local: whole, remote: whole, common: 3000, onlyA: 7, onlyB: 7, paired: true},
		{name: "spread over more than MaxParts ranges", local: whole, remote: whole, common: 1000, onlyA: 3000, onlyB: 3000},
		{name: "edge peer", local: status(rangeOf("01"), 12), remote: status(rangeOf("0110"), 12), common: 3000, onlyA: 300, onlyB: 300},
		{name: "initiator with the narrower root", local: status(rangeOf("0110"), 12), remote: status(rangeOf("01"), 12), common: 3000, onlyA: 300, onlyB: 300},
		{name: "misaligned roots", local: status(rangeOf("01"), 12), remote: status(rangeOf("011"), 12), common: 3000, onlyA: 300, onlyB: 300},
		{name: "differing leaf bits", local: status(rangeOf("01"), 12), remote: status(rangeOf("01"), 7), common: 3000, onlyA: 300, onlyB: 300},
		{name: "stripe opening", local: whole, remote: whole, common: 3000, onlyA: 300, onlyB: 300, open: []fingerprint.Range{rangeOf("0110")}},
		{name: "halves opening", local: whole, remote: whole, common: 3000, onlyA: 300, onlyB: 300, open: []fingerprint.Range{rangeOf("0"), rangeOf("1")}},
		{
			name: "opening deeper than the leaf depth", local: whole, remote: whole, common: 300, onlyA: 30, onlyB: 30,
			open: rangeOf("011011001100").Children(2), gen: rangePtr(rangeOf("011011001100")),
		},
		{
			name: "streams longer than a message on both sides", local: status(rangeOf("01"), 12), remote: status(rangeOf("01"), 0),
			common: 40000, onlyA: 5, onlyB: 5, longStreams: true,
		},
	} {
		for _, symbols := range []bool{false, true} {
			name := tc.name + "/phase1"
			if symbols {
				name = tc.name + "/symbols"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				rng := newRand(uint64(len(tc.name)))
				scope := mustScope(t, tc.local, tc.remote)
				gen := tc.local.Root
				if tc.gen != nil {
					gen = *tc.gen
				}
				open := tc.open
				if open == nil {
					open = []fingerprint.Range{scope.Root}
				}
				la, lb, a2b, b2a := convergenceSets(rng, gen, scope, tc.common, tc.onlyA, tc.onlyB, tc.paired)
				inOpen := func(ids []fingerprint.ID) []fingerprint.ID {
					var out []fingerprint.ID
					for _, id := range ids {
						for _, r := range open {
							if r.ContainsID(id) {
								out = append(out, id)
								break
							}
						}
					}
					return out
				}
				if tc.paired {
					if sa, _ := la.Summary(scope.Root); sa.Count != func() uint64 { s, _ := lb.Summary(scope.Root); return s.Count }() {
						t.Fatal("paired sets have different counts")
					}
				}

				cfgA := rangesync.Config{Scope: scope, Local: la, Open: tc.open}
				cfgB := rangesync.Config{Scope: mustScope(t, tc.remote, tc.local), Local: lb}
				if symbols {
					cfgA.Status, cfgA.PeerStatus = tc.local, tc.remote
					cfgB.Status, cfgB.PeerStatus = tc.remote, tc.local
					la.status, lb.status = &cfgA.Status, &cfgB.Status
				}
				a, first, err := rangesync.NewInitiator(cfgA)
				if err != nil {
					t.Fatal(err)
				}
				b := rangesync.NewResponder(cfgB)
				symbolParts := 0
				countSymbols := func(_ bool, m *pb.Message) {
					for _, p := range m.GetParts() {
						if p.GetSymbols() != nil {
							symbolParts++
						}
					}
				}
				wantsA, wantsB, messages := drive(t, a, first, b, pageChecker(t, new(int)), spanChecker(t, tc.longStreams && !symbols), countSymbols)
				assertSet(t, wantsA, inOpen(a2b))
				assertSet(t, wantsB, inOpen(b2a))
				assertDifference(t, a, b, len(inOpen(a2b))+len(inOpen(b2a)))
				if tc.onlyA+tc.onlyB == 0 && messages != 2 {
					t.Fatalf("equal sets took %d messages, want 2", messages)
				}
				bothHold := tc.common > 0 || tc.onlyA > 0 && tc.onlyB > 0
				if symbols && bothHold && tc.onlyA+tc.onlyB > 0 && symbolParts == 0 {
					t.Fatal("no symbol stream")
				}
				if !symbols && symbolParts != 0 {
					t.Fatalf("%d symbol parts without Statuses", symbolParts)
				}
				if c := a.Stats().DecodeCorruptions + b.Stats().DecodeCorruptions; c != 0 {
					t.Fatalf("%d corrupt decodes of honest streams", c)
				}
				t.Logf("%d messages, %d symbol parts, stats %+v / %+v", messages, symbolParts, a.Stats(), b.Stats())
			})
		}
	}
}

func rangePtr(r fingerprint.Range) *fingerprint.Range { return &r }

// spanChecker returns an inspect function that, when want is set, fails the
// test at its end unless some Items stream had pages in three messages.
func spanChecker(t *testing.T, want bool) func(bool, *pb.Message) {
	t.Helper()
	type key struct {
		from bool
		r    fingerprint.RangeKey
	}
	spans := map[key]int{}
	longest := 0
	if want {
		t.Cleanup(func() {
			if longest < 3 {
				t.Errorf("longest Items stream spans %d messages, want at least 3", longest)
			}
		})
	}
	return func(from bool, m *pb.Message) {
		seen := map[key]bool{}
		_, items := partsOf(t, m)
		for _, it := range items {
			k := key{from, it.Range.Key()}
			if !seen[k] {
				seen[k] = true
				spans[k]++
				longest = max(longest, spans[k])
			}
		}
	}
}

// TestSessionAnswerWaits is the review repro of an honest session that broke
// the empty-message rule: the answer stream has More set while it waits for
// the peer's next Reply page, and has nothing to send in the meantime.
func TestSessionAnswerWaits(t *testing.T) {
	t.Parallel()

	root := rangeOf("01")
	scope := rangesync.Scope{Root: root, LeafDepth: 2}
	ids := randIDs(newRand(99), root, 60000)
	slices.SortFunc(ids, fingerprint.ID.Compare)
	// an initiator-only ID right after the first common one
	extra := ids[0]
	extra[63]++
	if extra.Compare(ids[0]) <= 0 || extra.Compare(ids[1]) >= 0 {
		t.Fatal("extra id is not between the first two")
	}
	a, open, err := rangesync.NewInitiator(rangesync.Config{Scope: scope, Local: newFakeLocal(append(slices.Clone(ids), extra)...)})
	if err != nil {
		t.Fatal(err)
	}
	b := rangesync.NewResponder(rangesync.Config{Scope: scope, Local: newFakeLocal(ids...)})
	wantsA, wantsB, _ := drive(t, a, open, b, spanChecker(t, true))
	assertSet(t, wantsA, nil)
	assertSet(t, wantsB, []fingerprint.ID{extra})
}

// TestSessionNoEcho is the review repro of answers sending back IDs the peer
// listed: when the answers fit in the next message, none may be echoed.
func TestSessionNoEcho(t *testing.T) {
	t.Parallel()

	rng := newRand(5)
	scope := rangesync.Scope{Root: rangeOf(""), LeafDepth: 8}
	la, lb, a2b, b2a := convergenceSets(rng, scope.Root, scope, 20000, 1000, 1000, false)
	a, open, err := rangesync.NewInitiator(rangesync.Config{Scope: scope, Local: la})
	if err != nil {
		t.Fatal(err)
	}
	b := rangesync.NewResponder(rangesync.Config{Scope: scope, Local: lb})
	listedBy := [2]map[fingerprint.ID]bool{{}, {}}
	echoed := 0
	inspect := func(fromInitiator bool, m *pb.Message) {
		side := 0
		if fromInitiator {
			side = 1
		}
		_, items := partsOf(t, m)
		for _, it := range items {
			for _, id := range it.IDs {
				if it.Reply {
					listedBy[side][id] = true
				} else if listedBy[1-side][id] {
					echoed++
				}
			}
		}
	}
	wantsA, wantsB, _ := drive(t, a, open, b, inspect)
	assertSet(t, wantsA, a2b)
	assertSet(t, wantsB, b2a)
	if echoed != 0 {
		t.Fatalf("%d listed IDs sent back", echoed)
	}
}

// TestSessionExclusionCap runs an honest session whose answers do not fit in
// one message, so more than MaxPendingReplies answers stay pending: the
// session keeps at most that many exclusion lists and still converges.
func TestSessionExclusionCap(t *testing.T) {
	t.Parallel()

	rng := newRand(13)
	scope := rangesync.Scope{Root: rangeOf(""), LeafDepth: 8}
	leaves := scope.Root.Children(8)
	common := make([]fingerprint.ID, 0, 2*len(leaves))
	onlyA := make([]fingerprint.ID, 0, 100*len(leaves))
	for _, leaf := range leaves {
		common = append(common, randIDs(rng, leaf, 2)...)
		onlyA = append(onlyA, randIDs(rng, leaf, 100)...)
	}
	a, open, err := rangesync.NewInitiator(rangesync.Config{Scope: scope, Local: newFakeLocal(append(slices.Clone(common), onlyA...)...)})
	if err != nil {
		t.Fatal(err)
	}
	b := rangesync.NewResponder(rangesync.Config{Scope: scope, Local: newFakeLocal(common...)})
	most := 0
	inspect := func(fromInitiator bool, _ *pb.Message) {
		if !fromInitiator {
			return
		}
		withExcl, excluded, _ := a.PendingReplies()
		if withExcl > rangesync.MaxPendingReplies || excluded > rangesync.MaxPendingReplies*rangesync.ItemsPageSize {
			t.Fatalf("%d pending exclusion lists with %d IDs", withExcl, excluded)
		}
		most = max(most, withExcl)
	}
	wantsA, wantsB, _ := drive(t, a, open, b, inspect)
	assertSet(t, wantsA, nil)
	assertSet(t, wantsB, onlyA)
	// past MaxPendingReplies the answers carry IDs the peer listed; the
	// difference counts only the IDs one side lacks
	assertDifference(t, a, b, len(onlyA))
	if most != rangesync.MaxPendingReplies {
		t.Fatalf("at most %d pending exclusion lists, want the cap %d reached", most, rangesync.MaxPendingReplies)
	}
}
