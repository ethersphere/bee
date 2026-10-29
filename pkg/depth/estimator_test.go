// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package depth_test

import (
	"slices"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/depth"
)

func tenOf(d ...uint8) []uint8 { return d }

func TestEstimatorUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		from       uint8
		depths     []uint8
		then       []uint8
		wantDepth  uint8
		wantKnown  bool
		wantResult depth.UpdateResult
	}{
		{name: "unknown below five samples", depths: []uint8{9, 9, 9, 9}, wantKnown: false, wantResult: depth.UpdateTooFew},
		{name: "first value is the mode", depths: []uint8{9, 9, 10, 9, 10}, wantDepth: 9, wantKnown: true, wantResult: depth.UpdateFirst},
		{name: "first value tie goes lower", depths: []uint8{9, 9, 10, 10, 11}, wantDepth: 9, wantKnown: true, wantResult: depth.UpdateFirst},
		{name: "first value on the fifth sample from a poll", depths: []uint8{9, 9, 9, 9}, then: []uint8{10}, wantDepth: 9, wantKnown: true, wantResult: depth.UpdateFirst},
		{name: "five five tie keeps current", from: 9, depths: tenOf(9, 9, 9, 10, 9, 10, 10, 9, 10, 10), wantDepth: 9, wantKnown: true, wantResult: depth.UpdateNoMajority},
		{name: "six four flips", from: 9, depths: tenOf(9, 9, 10, 9, 10, 10, 9, 10, 10, 10), wantDepth: 10, wantKnown: true, wantResult: depth.UpdateStep},
		{name: "two level jump moves one", from: 9, depths: tenOf(11, 11, 11, 11, 11, 11, 11, 11, 11, 11), wantDepth: 10, wantKnown: true, wantResult: depth.UpdateStep},
		{name: "short window after known keeps value", from: 10, depths: []uint8{9, 9}, wantDepth: 10, wantKnown: true, wantResult: depth.UpdateTooFew},
		{name: "majority equal to estimate", from: 9, depths: tenOf(9, 9, 9, 9, 9, 9, 10, 10, 10, 10), wantDepth: 9, wantKnown: true, wantResult: depth.UpdateNone},
		{name: "step down", from: 10, depths: tenOf(9, 9, 9, 9, 9, 9, 10, 10, 10, 10), wantDepth: 9, wantKnown: true, wantResult: depth.UpdateStep},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			e := depth.NewEstimator()
			var block uint64
			add := func(d uint8) {
				block++
				if !e.Add(depth.Sample{Block: block, Depth: d}) {
					t.Fatalf("sample at block %d not added", block)
				}
			}
			if tc.from != 0 {
				for range depth.WindowSize {
					add(tc.from)
				}
				if got := e.Update(); got != depth.UpdateFirst {
					t.Fatalf("seed update: got %v want %v", got, depth.UpdateFirst)
				}
				e.Prune(1_000_000, 1)
				if e.Size() != 0 {
					t.Fatalf("window not cleared: %d", e.Size())
				}
			}
			for _, d := range tc.depths {
				add(d)
			}
			got := e.Update()
			if len(tc.then) > 0 {
				for _, d := range tc.then {
					add(d)
				}
				got = e.Update()
			}
			if got != tc.wantResult {
				t.Fatalf("result: got %v want %v", got, tc.wantResult)
			}
			d, known := e.Depth()
			if known != tc.wantKnown || (known && d != tc.wantDepth) {
				t.Fatalf("depth: got (%d, %v) want (%d, %v)", d, known, tc.wantDepth, tc.wantKnown)
			}
		})
	}
}

func TestEstimatorDedupe(t *testing.T) {
	t.Parallel()

	e := depth.NewEstimator()
	s := depth.Sample{Block: 5, TxIndex: 0, LogIndex: 1, Depth: 10}
	if !e.Add(s) {
		t.Fatal("first add returned false")
	}
	if e.Add(s) {
		t.Fatal("duplicate add returned true")
	}
	if e.Size() != 1 {
		t.Fatalf("size: got %d want 1", e.Size())
	}
}

func TestEstimatorInsertsOutOfOrder(t *testing.T) {
	t.Parallel()

	e := depth.NewEstimator()
	for _, b := range []uint64{5, 3, 4} {
		if !e.Add(depth.Sample{Block: b, Depth: 10}) {
			t.Fatalf("add block %d returned false", b)
		}
	}
	if got := e.Blocks(); !slices.Equal(got, []uint64{3, 4, 5}) {
		t.Fatalf("blocks: got %v, want [3 4 5]", got)
	}

	if !e.Add(depth.Sample{Block: 3, TxIndex: 1, Depth: 10}) {
		t.Fatal("add block 3 tx 1 returned false")
	}
	if got := e.Blocks(); !slices.Equal(got, []uint64{3, 3, 4, 5}) {
		t.Fatalf("blocks: got %v, want [3 3 4 5]", got)
	}
	// the second block 3 sits right after the first, so the tx-0 sample stays first
	if !e.Add(depth.Sample{Block: 3, TxIndex: 0, LogIndex: 1, Depth: 10}) {
		t.Fatal("add block 3 tx 0 log 1 returned false")
	}
	if got := e.Blocks(); !slices.Equal(got, []uint64{3, 3, 3, 4, 5}) {
		t.Fatalf("blocks: got %v, want [3 3 3 4 5]", got)
	}
}

func TestEstimatorRejectsDepthAboveMaxPO(t *testing.T) {
	t.Parallel()

	e := depth.NewEstimator()
	if e.Add(depth.Sample{Depth: 32}) {
		t.Fatal("depth 32 accepted")
	}
	if e.Size() != 0 {
		t.Fatalf("size: got %d want 0", e.Size())
	}
}

func TestEstimatorKeepsNewestTen(t *testing.T) {
	t.Parallel()

	e := depth.NewEstimator()
	for b := uint64(1); b <= 12; b++ {
		e.Add(depth.Sample{Block: b, Depth: 9})
	}
	blocks := e.Blocks()
	if len(blocks) != depth.WindowSize {
		t.Fatalf("size: got %d want %d", len(blocks), depth.WindowSize)
	}
	for i, b := range blocks {
		if want := uint64(i + 3); b != want {
			t.Fatalf("blocks[%d]: got %d want %d", i, b, want)
		}
	}
}

func TestEstimatorPrune(t *testing.T) {
	t.Parallel()

	const blocksPerRound = 152

	e := depth.NewEstimator()
	e.Add(depth.Sample{Block: 100, Depth: 9})
	e.Add(depth.Sample{Block: 4000, Depth: 9})
	e.Prune(4000, blocksPerRound)
	if got := e.Blocks(); len(got) != 1 || got[0] != 4000 {
		t.Fatalf("prune(4000): got %v want [4000]", got)
	}

	e = depth.NewEstimator()
	e.Add(depth.Sample{Block: 100, Depth: 9})
	e.Add(depth.Sample{Block: 4000, Depth: 9})
	e.Prune(1000, blocksPerRound)
	if got := e.Size(); got != 2 {
		t.Fatalf("prune(1000): size %d want 2", got)
	}
}
