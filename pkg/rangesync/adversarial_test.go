// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"encoding/binary"
	"slices"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
)

func symbolsPart(r fingerprint.Range) *pb.Part {
	return &pb.Part{Kind: &pb.Part_Symbols{Symbols: &pb.Symbols{Range: rangesync.EncodeRange(r), Count: 4, Data: make([]byte, 4*82)}}}
}

func morePart(r fingerprint.Range) *pb.Part {
	return &pb.Part{Kind: &pb.Part_More{More: &pb.More{Range: rangesync.EncodeRange(r), UpTo: 16}}}
}

func fpParts(rs []fingerprint.Range, s fingerprint.Summary) []*pb.Part {
	parts := make([]*pb.Part, len(rs))
	for i, r := range rs {
		parts[i] = fpPart(r, s)
	}
	return parts
}

func statusWithRoot(root fingerprint.Range) *pb.Status {
	return rangesync.EncodeStatus(rangesync.Status{Root: root, LeafBits: serviceLeafBits, Mapping: fingerprint.DefaultMapperID})
}

// TestReconcileAdversarial feeds a Phase-1 responder protocol violations over
// streamtest (proposal §6.4, §13): parts of Phase 2, ranges outside the
// session root or below its depth bound, children that do not partition a
// range the server sent, and oversized messages. Each one resets the stream,
// disconnects the peer and is scored once; no wants come out of the session.
func TestReconcileAdversarial(t *testing.T) {
	t.Parallel()

	whole := rangeOf("")
	differs := fingerprint.Summary{Count: 1, Digest: [32]byte{1}}
	open := mustMarshalDelimited(t, msg(fpPart(whole, differs)))
	// the server holds elements, so it answers open with the 16 children
	// of the root, at depth 4; an empty server asks for the peer's items
	pastCount := randIDs(newRand(7), whole, 2*rangesync.ItemsPageSize)
	slices.SortFunc(pastCount, fingerprint.ID.Compare)
	page1, page2 := pastCount[:rangesync.ItemsPageSize], pastCount[rangesync.ItemsPageSize:]
	for _, tc := range []struct {
		name string
		// empty runs the server with an empty reserve
		empty bool
		// root is the peer's Status root (default: the whole space)
		root *fingerprint.Range
		// first is written raw; second, if set, after the server's answer
		first, second []byte
	}{
		{name: "symbols in the opening", first: mustMarshalDelimited(t, msg(symbolsPart(whole)))},
		{name: "more in the opening", first: mustMarshalDelimited(t, msg(morePart(whole)))},
		{name: "symbols after a split", first: open, second: mustMarshalDelimited(t, msg(symbolsPart(rangeOf("0000"))))},
		{name: "more after a split", first: open, second: mustMarshalDelimited(t, msg(morePart(rangeOf("0000"))))},
		{
			name:   "range outside the session root",
			root:   rangePtr(rangeOf("01")),
			first:  mustMarshalDelimited(t, msg(fpPart(rangeOf("01"), differs))),
			second: mustMarshalDelimited(t, msg(fpParts(rangeOf("1").Children(fingerprint.FanoutBits), differs)...)),
		},
		{
			name:   "items outside the session root",
			root:   rangePtr(rangeOf("01")),
			first:  mustMarshalDelimited(t, msg(fpPart(rangeOf("01"), differs))),
			second: mustMarshalDelimited(t, msg(itemsPart(rangesync.Items{Range: rangeOf("10")}))),
		},
		{
			name:   "range below the depth bound",
			first:  open,
			second: mustMarshalDelimited(t, msg(fpParts(rangeOf("000000000000").Children(fingerprint.FanoutBits), differs)...)),
		},
		{
			name:   "split of a range the server did not send",
			first:  open,
			second: mustMarshalDelimited(t, msg(fpParts(rangeOf("00000000").Children(fingerprint.FanoutBits), differs)...)),
		},
		{
			name:   "incomplete split",
			first:  open,
			second: mustMarshalDelimited(t, msg(fpParts(rangeOf("0000").Children(fingerprint.FanoutBits)[1:], differs)...)),
		},
		{
			name:   "children of two levels",
			first:  open,
			second: mustMarshalDelimited(t, msg(fpParts(append(rangeOf("0000").Children(fingerprint.FanoutBits), rangeOf("0000").Children(2)...), differs)...)),
		},
		{
			name:   "too many parts",
			first:  open,
			second: mustMarshalDelimited(t, msg(fpParts(slices.Repeat(rangeOf("0000").Children(fingerprint.FanoutBits), rangesync.MaxParts/16+1), differs)...)),
		},
		{
			// the peer advertised 1 element: 1 + ItemsPageSize may follow
			name:  "items past the advertised count",
			empty: true,
			first: open,
			second: mustMarshalDelimited(t, msg(
				itemsPart(rangesync.Items{Range: whole, IDs: page1, More: true}),
				itemsPart(rangesync.Items{Range: whole, IDs: page2, After: idPtr(page1[len(page1)-1]), More: true}),
			)),
		},
		{
			name:   "oversized message",
			first:  open,
			second: binary.AppendUvarint(nil, rangesync.MaxMessageBytes+1),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := newTestNode(t, stampedChunks(t, 300)...)
			if tc.empty {
				b = newTestNode(t)
			}
			b.start(t)
			p := newRawPeer(t, b)
			root := whole
			if tc.root != nil {
				root = *tc.root
			}
			if err := p.sendStatus(t, statusWithRoot(root)); err != nil {
				t.Fatal(err)
			}
			s := p.open(t, rangesync.ReconcileStreamName)
			if _, err := s.Write(tc.first); err != nil {
				t.Fatal(err)
			}
			if tc.second != nil {
				var answer pb.Message
				if err := protobuf.NewReader(s).ReadMsg(&answer); err != nil {
					t.Fatal(err)
				}
				if answer.Busy || len(answer.Parts) == 0 {
					t.Fatalf("server did not answer with parts: %v", &answer)
				}
				if _, err := s.Write(tc.second); err != nil {
					t.Fatal(err)
				}
			}
			records := p.records(t, rangesync.ReconcileStreamName)
			_ = s.Reset()
			assertDisconnect(t, records[0].Err())
			assertFailed(t, b.scorer, p.addr)
			if p.resets.Load() == 0 {
				t.Fatal("stream not reset")
			}
			if len(b.responderWants(p.addr)) != 0 {
				t.Fatal("wants from a failed session")
			}
		})
	}
}

// TestReconcileOpeningOutsideRoot checks that an opening outside the session
// root is answered with Busy and not scored: one of the two Statuses may be
// stale.
func TestReconcileOpeningOutsideRoot(t *testing.T) {
	t.Parallel()

	b := newTestNode(t, stampedChunks(t, 30)...)
	b.start(t)
	p := newRawPeer(t, b)
	if err := p.sendStatus(t, statusWithRoot(rangeOf("01"))); err != nil {
		t.Fatal(err)
	}
	s := p.open(t, rangesync.ReconcileStreamName)
	if err := protobuf.NewWriter(s).WriteMsg(msg(fpPart(rangeOf("1"), fingerprint.Summary{Count: 1, Digest: [32]byte{1}}))); err != nil {
		t.Fatal(err)
	}
	var answer pb.Message
	if err := protobuf.NewReader(s).ReadMsg(&answer); err != nil {
		t.Fatal(err)
	}
	if !answer.Busy {
		t.Fatalf("answer %v, want busy", &answer)
	}
	_ = s.FullClose()
	if err := p.records(t, rangesync.ReconcileStreamName)[0].Err(); err != nil {
		t.Fatal(err)
	}
	assertNotScored(t, b, p.addr)
}

// TestReconcileEndlessItemsPaging checks the bound on a peer that keeps
// paging Items: within the count it advertised for the range (plus one page)
// every page is answered with an empty Message and the IDs reach
// Options.Wants; past it, or with a page that does not continue the stream,
// the session is a violation.
func TestReconcileEndlessItemsPaging(t *testing.T) {
	t.Parallel()

	const pages = 64
	whole := rangeOf("")
	rng := newRand(13)
	ids := randIDs(rng, whole, pages*rangesync.ItemsPageSize)
	slices.SortFunc(ids, fingerprint.ID.Compare)
	ids = slices.Compact(ids)

	for _, tc := range []struct {
		name string
		// advertised is the count of the opening Fingerprint
		advertised int
		broken     bool
		// violationAt is the page past the limit (-1: none)
		violationAt int
	}{
		{name: "within the advertised count", advertised: len(ids), violationAt: -1},
		{name: "past the advertised count", advertised: 4 * rangesync.ItemsPageSize, violationAt: 5},
		{name: "page that does not continue", advertised: len(ids), broken: true, violationAt: pages / 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// an empty server asks the peer for its items
			b := newTestNode(t)
			b.start(t)
			p := newRawPeer(t, b)
			if err := p.sendStatus(t, statusWithRoot(whole)); err != nil {
				t.Fatal(err)
			}
			s := p.open(t, rangesync.ReconcileStreamName)
			w := protobuf.NewWriter(s)
			read := func() *pb.Message {
				t.Helper()
				var m pb.Message
				if err := protobuf.NewReader(s).ReadMsg(&m); err != nil {
					t.Fatal(err)
				}
				return &m
			}
			if err := w.WriteMsg(msg(fpPart(whole, fingerprint.Summary{Count: uint64(tc.advertised), Digest: [32]byte{1}}))); err != nil {
				t.Fatal(err)
			}
			if _, items := partsOf(t, read()); len(items) != 1 || !items[0].Reply || len(items[0].IDs) != 0 {
				t.Fatalf("server answer: %v", items)
			}

			var after *fingerprint.ID
			sent := 0
			for i := 0; sent < len(ids); i++ {
				page := ids[sent:min(sent+rangesync.ItemsPageSize, len(ids))]
				last := sent+len(page) == len(ids)
				if tc.broken && i == tc.violationAt {
					after = idPtr(ids[0])
				}
				if err := w.WriteMsg(msg(itemsPart(rangesync.Items{Range: whole, IDs: page, After: after, More: !last}))); err != nil {
					t.Fatal(err)
				}
				sent += len(page)
				after = idPtr(page[len(page)-1])
				if i == tc.violationAt || last {
					break
				}
				if m := read(); len(m.Parts) != 0 || m.Busy {
					t.Fatalf("page %d answered with %v", i, m)
				}
			}
			_ = s.Close()
			records := p.records(t, rangesync.ReconcileStreamName)
			if tc.violationAt >= 0 {
				assertDisconnect(t, records[0].Err())
				assertFailed(t, b.scorer, p.addr)
				if len(b.responderWants(p.addr)) != 0 {
					t.Fatal("wants from a failed session")
				}
				return
			}
			if err := records[0].Err(); err != nil {
				t.Fatal(err)
			}
			if got := len(b.responderWants(p.addr)); got != len(ids) {
				t.Fatalf("got %d wants, want %d", got, len(ids))
			}
		})
	}
}
