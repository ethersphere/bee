// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// exampleSession is the outcome of one worked-example session.
type exampleSession struct {
	kinds    partKinds
	messages int // reconcile Messages with at least one part
	bytes    int // their wire bytes
	a, b     *rangesync.Session
}

// runExample runs a session between initiator A over common+onlyA and
// responder B over common+onlyB, both with a full-depth root Status, checks
// that each side learns exactly what it lacks and returns its cost.
func runExample(t *testing.T, common, onlyA, onlyB []fingerprint.ID) exampleSession {
	t.Helper()
	la := newFakeLocal(append(slices.Clone(common), onlyA...)...)
	lb := newFakeLocal(append(slices.Clone(common), onlyB...)...)
	a, b, first := symbolsPair(t, status(rangeOf(""), fingerprint.LeafBits), la, lb)
	var ex exampleSession
	cost := func(_ bool, m *pb.Message) {
		if len(m.GetParts()) == 0 {
			return
		}
		ex.messages++
		n := m.Size()
		ex.bytes += n + varintLen(n)
	}
	wantsA, wantsB, _ := drive(t, a, first, b, ex.kinds.inspect, cost)
	assertSet(t, wantsA, onlyB)
	assertSet(t, wantsB, onlyA)
	ex.a, ex.b = a, b
	return ex
}

func varintLen(n int) int {
	l := 1
	for ; n >= 0x80; n >>= 7 {
		l++
	}
	return l
}

// within20 reports whether got is within ±20% of want.
func within20(got, want int) bool {
	return float64(got) >= 0.8*float64(want) && float64(got) <= 1.2*float64(want)
}

// restampedID returns the ID of the chunk of id under another stamp.
func restampedID(id fingerprint.ID) fingerprint.ID {
	stamp := make([]byte, swarm.HashSize)
	stamp[0] = 1
	return fingerprint.NewID(id.Address(), stamp)
}

// outcome is a message count and the metadata bytes they carry.
type outcome struct {
	messages, bytes int
}

// TestWorkedExamples runs the sessions of proposal §7.10 (examples 1–3) and
// §7.2/§7.3 literally and checks the reconcile Messages (those with parts;
// the closing empty Message is not counted, as in the proposal) and their
// metadata bytes, within ±20%, against the numbers the proposal states.
//
// Where the implementation differs from the proposal's numbers, the case
// lists the measured outcome too, with the reason (T2.6 Decisions log). The
// symbol streams grow by GrowthFactor 2 since T4.7 part 2 (G6); the
// proposal's text of examples 2 and 3 and §7.2 still has GrowthFactor 4's
// More requests, so those cases take GrowthFactor 2's outcomes (the phase-4
// Follow-ups list the proposal text to update):
//
//   - Example 2: a first batch of 68 symbols decodes 45 differences only
//     about half of the time (the median stream needs 68 symbols, not ~62),
//     otherwise one More(root, UpTo 136) follows (the proposal's UpTo 272
//     is GrowthFactor 4's), and for a few draws a second one (UpTo 272).
//   - Example 3: the stream grows 4, 8, 16, 32, 64: 11 Messages with parts
//     (3 plus 4 round trips of More and Symbols; the proposal's own sequence,
//     UpTo 16 and 64, has GrowthFactor 4's 7, not 5), or 9 when 32 symbols
//     decode the 25 differences.
//   - §7.2: the counts N+1 and N+2 give d̂ = 1, not 3, so n0 = 4 (not 7):
//     ~0.46 KB when 4 symbols decode, else one More(root, UpTo 8).
//   - §7.3: the split sends A's 16 streams and B's 16 Items parts.
func TestWorkedExamples(t *testing.T) {
	t.Parallel()

	const (
		n      = fingerprint.ScanLimit + 1000 // the root is served from its cache
		fp     = 55                           // an opening Message with FP(root)
		symbol = 82                           // one coded symbol on the wire
		item   = 70                           // one ID in an Items part
	)
	root := rangeOf("")
	for _, tc := range []struct {
		name  string
		seeds int
		build func(rng *rand.Rand) (common, onlyA, onlyB []fingerprint.ID)
		// want is the proposal's outcome (GrowthFactor 2's where the case
		// list above says so); also are the other outcomes the
		// implementation takes for some draws of the sets
		want outcome
		also []outcome
	}{
		{
			name:  "example 1: one chunk missing",
			seeds: 5,
			build: func(rng *rand.Rand) ([]fingerprint.ID, []fingerprint.ID, []fingerprint.ID) {
				return randIDs(rng, root, n), randIDs(rng, root, 1), nil
			},
			want: outcome{3, fp + 4*symbol + item},
		},
		{
			name:  "example 2: several kinds of difference",
			seeds: 10,
			build: func(rng *rand.Rand) ([]fingerprint.ID, []fingerprint.ID, []fingerprint.ID) {
				x := randID(rng, rangeOf("0011"))
				z := randID(rng, rangeOf("1100"))
				onlyB := append(randIDs(rng, rangeOf("1010"), 2), randIDs(rng, rangeOf("0111"), 40)...)
				return randIDs(rng, root, n), []fingerprint.ID{x, z}, append(onlyB, restampedID(z))
			},
			want: outcome{3, fp + 68*symbol + 2*item},
			also: []outcome{{5, fp + 136*symbol + 2*item}, {7, fp + 272*symbol + 2*item}},
		},
		{
			name:  "example 3: equal counts hide a larger difference",
			seeds: 5,
			build: func(rng *rand.Rand) ([]fingerprint.ID, []fingerprint.ID, []fingerprint.ID) {
				return randIDs(rng, root, n), randIDs(rng, root, 12), randIDs(rng, root, 13)
			},
			want: outcome{11, 64*symbol + 12*item},
			also: []outcome{{9, 32*symbol + 12*item}},
		},
		{
			name:  "7.2: small difference from the root",
			seeds: 10,
			build: func(rng *rand.Rand) ([]fingerprint.ID, []fingerprint.ID, []fingerprint.ID) {
				return randIDs(rng, root, n), randIDs(rng, root, 1), randIDs(rng, root, 2)
			},
			want: outcome{3, fp + 4*symbol + item},
			also: []outcome{{5, fp + 8*symbol + item}},
		},
		{
			name:  "7.3: one split, then level-1 streams",
			seeds: 1,
			build: func(rng *rand.Rand) ([]fingerprint.ID, []fingerprint.ID, []fingerprint.ID) {
				// every level-1 range past ScanLimit, so that it has a cache
				return randIDs(rng, root, 16*(fingerprint.ScanLimit+400)), randIDs(rng, root, 4000), nil
			},
			want: outcome{4, 16 * 402 * symbol},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			seen := map[outcome]int{}
			for seed := range tc.seeds {
				common, onlyA, onlyB := tc.build(newRand(uint64(seed)))
				ex := runExample(t, common, onlyA, onlyB)
				ok := false
				for _, o := range append([]outcome{tc.want}, tc.also...) {
					if ex.messages == o.messages && within20(ex.bytes, o.bytes) {
						seen[o]++
						ok = true
						break
					}
				}
				if !ok {
					t.Errorf("seed %d: %d messages, %d bytes (%v), want %v or one of %v (bytes ± 20%%)", seed, ex.messages, ex.bytes, ex.kinds.msgs, tc.want, tc.also)
				}
				if c := ex.a.Stats().DecodeCorruptions + ex.b.Stats().DecodeCorruptions; c != 0 {
					t.Errorf("seed %d: %d corrupt decodes", seed, c)
				}
			}
			if seen[tc.want] == 0 {
				t.Errorf("the proposal's outcome %v never taken: %v", tc.want, seen)
			}
			t.Logf("outcomes %v", seen)
		})
	}
}
