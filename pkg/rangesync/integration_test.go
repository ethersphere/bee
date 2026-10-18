// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"context"
	"testing"

	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// meyerBounds are the Phase-1 cost bounds of one session between two sets
// (proposal §7, Meyer descent with item lists): a range is split only where
// the sets differ, and items are listed only in ranges where they differ.
type meyerBounds struct {
	messages int
	bytes    int
}

// computeMeyerBounds returns the bounds of a session over the whole address
// space (root of 0 bits) with leaf depth leafDepth between the sets a and b.
//
//   - Fingerprints: the opening, and 16 children for each range above the
//     leaf depth that holds an element of the symmetric difference.
//   - Items: every ID in a leaf range that holds an element of the symmetric
//     difference, listed at most once by its holder, plus one page header
//     per page and range.
//   - Messages: two per split level, the opening, the Reply exchange and the
//     closing message, plus two per message worth of Items pages.
func computeMeyerBounds(a, b []fingerprint.ID, leafDepth int) meyerBounds {
	const (
		// fpBytes bounds one Fingerprint part with its Part wrapper.
		fpBytes = 3 + 2 + (2 + 3 + 2) + (1 + 10) + (2 + 32)
		// idBytes bounds one ID in an Items part.
		idBytes = 2 + 64
		// pageBytes bounds the header of one Items page.
		pageBytes = 3 + (2 + 7) + (2 + 2 + 64) + 2 + 2
		// messageBytes bounds the envelope of a Message on the wire.
		messageBytes = 5 + 3
	)
	inA := make(map[fingerprint.ID]bool, len(a))
	for _, id := range a {
		inA[id] = true
	}
	inB := make(map[fingerprint.ID]bool, len(b))
	for _, id := range b {
		inB[id] = true
	}
	prefix := func(id fingerprint.ID, bits int) string {
		r := fingerprint.NewRange(id.Address(), uint8(bits))
		return string(r.Prefix.Bytes())
	}
	// ranges[l] holds the l-bit prefixes of the symmetric difference
	ranges := map[int]map[string]bool{}
	for l := 0; l <= leafDepth; l += fingerprint.FanoutBits {
		ranges[l] = map[string]bool{}
	}
	mark := func(id fingerprint.ID) {
		for l := range ranges {
			ranges[l][prefix(id, l)] = true
		}
	}
	for _, id := range a {
		if !inB[id] {
			mark(id)
		}
	}
	for _, id := range b {
		if !inA[id] {
			mark(id)
		}
	}
	listed := 0
	for _, ids := range [][]fingerprint.ID{a, b} {
		for _, id := range ids {
			if ranges[leafDepth][prefix(id, leafDepth)] {
				listed++
			}
		}
	}
	splits := 0
	for l := 0; l < leafDepth; l += fingerprint.FanoutBits {
		splits += len(ranges[l])
	}
	leaves := len(ranges[leafDepth])
	pages := 2 * (leaves + listed/rangesync.ItemsPageSize + 1)
	itemBytes := listed*idBytes + pages*pageBytes
	itemMessages := 2 * (itemBytes/(rangesync.MaxMessageBytes/2) + 1)
	messages := 2*(leafDepth/fingerprint.FanoutBits) + 3 + itemMessages
	return meyerBounds{
		messages: messages,
		bytes:    (1+splits<<fingerprint.FanoutBits)*fpBytes + itemBytes + messages*messageBytes,
	}
}

func fpIDs(tb testing.TB, chs []swarm.Chunk) []fingerprint.ID {
	tb.Helper()
	ids := make([]fingerprint.ID, len(chs))
	for i, ch := range chs {
		ids[i] = fpID(chunkIDOf(tb, ch))
	}
	return ids
}

// restamped returns copies of chs with new stamps: the same addresses, other
// IDs (a stale-stamp divergence).
func restamped(chs []swarm.Chunk) []swarm.Chunk {
	out := make([]swarm.Chunk, len(chs))
	for i, ch := range chs {
		out[i] = swarm.NewChunk(ch.Address(), ch.Data()).WithStamp(postagetesting.MustNewStamp())
	}
	return out
}

// TestProtocolConverges runs a reconcile session between two services over
// streamtest for differences of 0, 1, 20, 1k, 4k and 100% (an empty side)
// and a hidden difference (equal counts at every level), checks that each
// side learns exactly the IDs it lacks, that message and byte counts stay
// within the Meyer bounds plus an allowance for symbol streams, and that the metrics match the wire. For the
// small differences the wants are fetched and both reserves end with the
// union; a second session then finds the sets equal.
func TestProtocolConverges(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name                 string
		common, onlyA, onlyB int
		// hidden gives B a restamped copy of each A-only chunk: equal
		// counts in every range
		hidden bool
		// fetch fetches the wants with valid chunks (the delivery rate is
		// limited to 250 chunks per second per peer)
		fetch bool
	}{
		{name: "0", common: 4096, fetch: true},
		{name: "1", common: 4096, onlyA: 1, fetch: true},
		{name: "20", common: 4096, onlyA: 10, onlyB: 10, fetch: true},
		{name: "hidden", common: 4096, onlyA: 10, hidden: true, fetch: true},
		{name: "1k", common: 4096, onlyA: 500, onlyB: 500},
		{name: "4k", common: 4096, onlyA: 2048, onlyB: 2048},
		{name: "100%", onlyB: 4096},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			only := stampedChunks
			if tc.fetch {
				only = func(_ testing.TB, n int) []swarm.Chunk { return validChunks(n) }
			}
			common := stampedChunks(t, tc.common)
			onlyA := only(t, tc.onlyA)
			onlyB := only(t, tc.onlyB)
			if tc.hidden {
				onlyB = restamped(onlyA)
			}
			setA := append(append([]swarm.Chunk{}, common...), onlyA...)
			setB := append(append([]swarm.Chunk{}, common...), onlyB...)
			a := newTestNode(t, setA...)
			b := newTestNode(t, setB...)
			a.start(t)
			b.start(t)
			connect(t, a, b)
			if tc.hidden {
				sa, _ := a.svc.LocalStatus()
				sb, _ := b.svc.LocalStatus()
				if sa.Summary.Count != sb.Summary.Count || sa.Summary == sb.Summary {
					t.Fatal("hidden difference is not hidden")
				}
			}

			wants, err := a.svc.Reconcile(context.Background(), b.addr)
			if err != nil {
				t.Fatal(err)
			}
			sent, received := reconcileMessages(t, a, b.addr)
			assertChunkIDs(t, wants, idsOf(t, onlyB...)...)
			assertChunkIDs(t, b.responderWants(a.addr), idsOf(t, onlyA...)...)

			// cost within the Meyer bounds
			bound := computeMeyerBounds(fpIDs(t, setA), fpIDs(t, setB), serviceLeafBits)
			// Symbol streams (Phase 2) can cost more than the Meyer bound
			// when the count estimate is low (equal counts give d̂ = 1): each
			// More is one extra round trip, 5 from MinBatch to RootSymbols
			// and 4 to Level1Symbols, and a stream that is exhausted before
			// it decodes wastes its symbols (the root and the 16 level-1
			// streams at most). T2.6 asserts the worked examples literally.
			bound.messages += 2 * (5 + 4)
			bound.bytes += (fingerprint.RootSymbols + 16*fingerprint.Level1Symbols) * fingerprint.MaxSymbolSize
			messages := len(sent) + len(received)
			bytes := 0
			for _, m := range append(append([]*pb.Message{}, sent...), received...) {
				bytes += wireSize(t, m)
			}
			t.Logf("diff %d: %d messages (bound %d), %d bytes (bound %d)", tc.onlyA+tc.onlyB, messages, bound.messages, bytes, bound.bytes)
			if messages > bound.messages {
				t.Errorf("%d messages, bound %d", messages, bound.messages)
			}
			if bytes > bound.bytes {
				t.Errorf("%d bytes, bound %d", bytes, bound.bytes)
			}
			if tc.onlyA+tc.onlyB == 0 && messages != 2 {
				t.Errorf("equal sets took %d messages, want 2", messages)
			}

			assertSessionMetrics(t, a, b, sent, received, len(wants))

			if !tc.fetch {
				return
			}
			for _, x := range []struct {
				from, to *testNode
				ids      []storer.ChunkID
			}{{a, b, wants}, {b, a, b.responderWants(a.addr)}} {
				if len(x.ids) == 0 {
					continue
				}
				stored, err := x.from.svc.Fetch(context.Background(), x.to.addr, x.ids)
				if err != nil {
					t.Fatal(err)
				}
				if stored != len(x.ids) {
					t.Fatalf("stored %d, want %d", stored, len(x.ids))
				}
			}
			union := idsOf(t, append(setA[:len(setA):len(setA)], onlyB...)...)
			assertStored(t, a, true, union...)
			assertStored(t, b, true, union...)
			assertChunkBytes(t, a, b)

			// the indexes caught up: a second session finds equal sets
			waitFor(t, "equal roots", func() bool {
				sa, errA := a.svc.LocalStatus()
				sb, errB := b.svc.LocalStatus()
				return errA == nil && errB == nil && sa.Summary == sb.Summary
			})
			wants, err = a.svc.Reconcile(context.Background(), b.addr)
			if err != nil {
				t.Fatal(err)
			}
			if len(wants) != 0 {
				t.Fatalf("second session: %d wants", len(wants))
			}
		})
	}
}

// wireSize is the size of m on a stream: its encoding and length prefix.
func wireSize(t *testing.T, m *pb.Message) int {
	t.Helper()
	return len(mustMarshalDelimited(t, m))
}

// assertSessionMetrics checks the reconcile metrics of initiator a and
// responder b after one session in which a sent sent and received received.
func assertSessionMetrics(t *testing.T, a, b *testNode, sent, received []*pb.Message, wantsA int) {
	t.Helper()
	count := func(ms []*pb.Message) map[string]float64 {
		c := map[string]float64{}
		for _, m := range ms {
			for _, p := range m.Parts {
				switch {
				case p.GetFP() != nil:
					c["fingerprint"]++
				case p.GetItems() != nil:
					c["items"]++
				case p.GetMore() != nil:
					c["more"]++
				case p.GetSymbols() != nil:
					c["symbols"]++
				}
			}
		}
		return c
	}
	cs, cr := count(sent), count(received)
	for _, typ := range []string{"fingerprint", "items"} {
		if got := a.svc.Parts(true, typ); got != cs[typ] {
			t.Errorf("a parts_sent{%s}: %v, want %v", typ, got, cs[typ])
		}
		if got := a.svc.Parts(false, typ); got != cr[typ] {
			t.Errorf("a parts_received{%s}: %v, want %v", typ, got, cr[typ])
		}
		if got := b.svc.Parts(false, typ); got != cs[typ] {
			t.Errorf("b parts_received{%s}: %v, want %v", typ, got, cs[typ])
		}
	}
	if got := a.svc.MoreRequests(); got != cs["more"] {
		t.Errorf("a more_requests_total: %v, want %v", got, cs["more"])
	}
	if got := b.svc.MoreRequests(); got != cr["more"] {
		t.Errorf("b more_requests_total: %v, want %v", got, cr["more"])
	}
	// one observation per stream of b decoded by a with a difference, at
	// least one symbol per difference
	if n, sum := a.svc.SymbolsPerDifference(); n > uint64(cr["symbols"]) || sum < float64(n) || cr["symbols"] > 0 && wantsA > 0 && n == 0 {
		t.Errorf("a symbols_per_difference: %d observations, sum %v, %v symbol parts received", n, sum, cr["symbols"])
	}
	if n, sum := a.svc.SessionRounds(); n != 1 || int(sum) != len(received) {
		t.Errorf("a session_rounds: %d sessions, sum %v, want 1, %d", n, sum, len(received))
	}
	if n, _ := b.svc.SessionRounds(); n != 1 {
		t.Errorf("b session_rounds: %d sessions", n)
	}
	if n, sum := a.svc.PairwiseDiff(); n != 1 || int(sum) != wantsA {
		t.Errorf("a pairwise_diff: %d sessions, sum %v, want 1, %d", n, sum, wantsA)
	}
	if _, sum := b.svc.PairwiseDiff(); int(sum) != len(b.responderWants(a.addr)) {
		t.Errorf("b pairwise_diff: sum %v, want %d", sum, len(b.responderWants(a.addr)))
	}
	// metadata out of a: its Status and the Messages it sent
	statusBytes := 0
	records, err := a.rec.Records(b.addr, rangesync.ProtocolName, rangesync.ProtocolVersion, rangesync.StatusStreamName)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range records {
		statusBytes += len(r.In())
	}
	sentBytes := 0
	for _, m := range sent {
		sentBytes += wireSize(t, m)
	}
	if got := a.svc.MetadataBytes("out"); int(got) != statusBytes+sentBytes {
		t.Errorf("a bytes_metadata{out}: %v, want %d", got, statusBytes+sentBytes)
	}
}

// assertChunkBytes checks bytes_chunks{in} of a against the deliveries a
// read on its fetch streams to b, and bytes_chunks{out} of b against them.
func assertChunkBytes(t *testing.T, a, b *testNode) {
	t.Helper()
	// no fetch stream when nothing was wanted
	records, _ := a.rec.Records(b.addr, rangesync.ProtocolName, rangesync.ProtocolVersion, rangesync.FetchStreamName)
	n := 0
	for _, r := range records {
		n += len(r.Out())
	}
	if got := a.svc.ChunkBytes("in"); int(got) != n {
		t.Errorf("a bytes_chunks{in}: %v, want %d", got, n)
	}
	if got := b.svc.ChunkBytes("out"); int(got) != n {
		t.Errorf("b bytes_chunks{out}: %v, want %d", got, n)
	}
}
