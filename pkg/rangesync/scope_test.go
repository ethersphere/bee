// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestSessionRoot covers the edge-peer cases of proposal §7.6.
func TestSessionRoot(t *testing.T) {
	t.Parallel()

	// 0110/4 with every bit past the prefix set
	garbled := fingerprint.Range{Prefix: swarm.NewAddress(bytes.Repeat([]byte{0x6f}, 32)), Bits: 4}
	for _, tc := range []struct {
		name string
		a, b fingerprint.Range
		want fingerprint.Range
		ok   bool
	}{
		{"equal", rangeOf("0110"), rangeOf("0110"), rangeOf("0110"), true},
		{"a contains b", rangeOf("01"), rangeOf("0110"), rangeOf("0110"), true},
		{"b contains a", rangeOf("0110"), rangeOf("01"), rangeOf("0110"), true},
		{"siblings", rangeOf("0110"), rangeOf("0111"), fingerprint.Range{}, false},
		{"disjoint at different depths", rangeOf("00"), rangeOf("0110"), fingerprint.Range{}, false},
		{"whole space", rangeOf(""), rangeOf("101"), rangeOf("101"), true},
		{"both whole space", rangeOf(""), rangeOf(""), rangeOf(""), true},
		{"non-canonical input", rangeOf("01"), garbled, rangeOf("0110"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := rangesync.SessionRoot(tc.a, tc.b)
			if ok != tc.ok {
				t.Fatalf("ok %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if !got.Equal(tc.want) || !bytes.Equal(got.Prefix.Bytes(), tc.want.Prefix.Bytes()) {
				t.Fatalf("got %v (%x), want %v", got, got.Prefix.Bytes(), tc.want)
			}
		})
	}
}

func TestNewScope(t *testing.T) {
	t.Parallel()

	long := func(bits int) fingerprint.Range {
		return fingerprint.NewRange(swarm.NewAddress(bytes.Repeat([]byte{0xff}, 32)), uint8(bits))
	}
	for _, tc := range []struct {
		name          string
		local, remote rangesync.Status
		root          fingerprint.Range
		leafDepth     uint8
		err           error
	}{
		{"same root", status(rangeOf("01"), 12), status(rangeOf("01"), 12), rangeOf("01"), 14, nil},
		{"edge peer, leaf depth is the smaller", status(rangeOf("01"), 12), status(rangeOf("0110"), 8), rangeOf("0110"), 12, nil},
		{"narrower local root", status(rangeOf("0110"), 12), status(rangeOf("01"), 12), rangeOf("0110"), 14, nil},
		{"disjoint", status(rangeOf("01"), 12), status(rangeOf("10"), 12), fingerprint.Range{}, 0, rangesync.ErrDisjointRoots},
		{"root deeper than leaf depth", status(rangeOf("01"), 1), status(rangeOf("0110"), 12), fingerprint.Range{}, 0, rangesync.ErrInvalidScope},
		{"root at leaf depth", status(rangeOf("01"), 2), status(rangeOf("0110"), 12), rangeOf("0110"), 4, nil},
		{"status past max bits", status(long(250), 10), status(long(250), 1), fingerprint.Range{}, 0, rangesync.ErrInvalidScope},
		{"remote past max bits", status(long(250), 1), status(long(250), 6), fingerprint.Range{}, 0, rangesync.ErrInvalidScope},
		{"leaf depth at max bits", status(long(200), 55), status(long(200), 55), long(200), 255, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, err := rangesync.NewScope(tc.local, tc.remote)
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("got %v, want %v", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !s.Root.Equal(tc.root) || s.LeafDepth != tc.leafDepth {
				t.Fatalf("got %v leaf depth %d, want %v leaf depth %d", s.Root, s.LeafDepth, tc.root, tc.leafDepth)
			}
		})
	}
}

func TestScopeCheckRange(t *testing.T) {
	t.Parallel()

	s := rangesync.Scope{Root: rangeOf("01"), LeafDepth: 14}
	for _, tc := range []struct {
		name  string
		r     fingerprint.Range
		valid bool
	}{
		{"root", rangeOf("01"), true},
		{"inside", rangeOf("0110"), true},
		{"outside", rangeOf("10"), false},
		{"containing the root", rangeOf("0"), false},
		{"whole space", rangeOf(""), false},
		{"at leaf depth", rangeOf("01101100110011"), true},
		{"at leaf depth plus slack", rangeOf("011011001100110000"), true},
		{"one past the slack", rangeOf("0110110011001100001"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := s.CheckRange(tc.r)
			if tc.valid && err != nil {
				t.Fatal(err)
			}
			if !tc.valid && !isInvalid(err) {
				t.Fatalf("got %v, want invalid", err)
			}
		})
	}

	// the slack does not wrap past MaxBits
	deep := rangesync.Scope{Root: fingerprint.NewRange(swarm.NewAddress(bytes.Repeat([]byte{0xff}, 32)), 250), LeafDepth: 254}
	if err := deep.CheckRange(fingerprint.NewRange(swarm.NewAddress(bytes.Repeat([]byte{0xff}, 32)), 255)); err != nil {
		t.Fatal(err)
	}
}

func TestScopeCheckMessage(t *testing.T) {
	t.Parallel()

	s := rangesync.Scope{Root: rangeOf("01"), LeafDepth: 14}
	in := fpPart(rangeOf("0110"), fingerprint.Summary{})
	out := fpPart(rangeOf("10"), fingerprint.Summary{})
	items := itemsPart(rangesync.Items{Range: rangeOf("011")})
	// full pages over n sub-ranges: 14 fit in MaxMessageBytes, 16 do not
	rng := newRand(14)
	fullPages := func(n int) []*pb.Part {
		parts := make([]*pb.Part, 0, n)
		for _, r := range rangeOf("01").Children(4)[:n] {
			ids := randIDs(rng, r, rangesync.ItemsPageSize)
			slicesSort(ids)
			parts = append(parts, itemsPart(rangesync.Items{Range: r, IDs: ids}))
		}
		return parts
	}
	many := func(n int) []*pb.Part {
		parts := make([]*pb.Part, n)
		for i := range parts {
			parts[i] = in
		}
		return parts
	}
	for _, tc := range []struct {
		name  string
		m     *pb.Message
		valid bool
	}{
		{"nil", nil, false},
		{"empty", msg(), true},
		{"fingerprint and items", msg(in, items), true},
		{"max parts", msg(many(rangesync.MaxParts)...), true},
		{"over max parts", msg(many(rangesync.MaxParts + 1)...), false},
		{"near max bytes", msg(fullPages(14)...), true},
		{"over max bytes", msg(fullPages(16)...), false},
		{"nil part", msg(in, nil), false},
		{"part without kind", msg(&pb.Part{}), false},
		{"fingerprint without body", msg(&pb.Part{Kind: &pb.Part_FP{}}), false},
		{"symbols", msg(&pb.Part{Kind: &pb.Part_Symbols{Symbols: &pb.Symbols{Range: rangesync.EncodeRange(rangeOf("01"))}}}), true},
		{"more", msg(&pb.Part{Kind: &pb.Part_More{More: &pb.More{Range: rangesync.EncodeRange(rangeOf("01"))}}}), true},
		{"symbols without body", msg(&pb.Part{Kind: &pb.Part_Symbols{}}), false},
		{"more without range", msg(&pb.Part{Kind: &pb.Part_More{More: &pb.More{}}}), false},
		{"symbols out of scope", msg(&pb.Part{Kind: &pb.Part_Symbols{Symbols: &pb.Symbols{Range: rangesync.EncodeRange(rangeOf("1"))}}}), false},
		{"more out of scope", msg(&pb.Part{Kind: &pb.Part_More{More: &pb.More{Range: rangesync.EncodeRange(rangeOf("1"))}}}), false},
		{"fingerprint out of scope", msg(in, out), false},
		{"items out of scope", msg(itemsPart(rangesync.Items{Range: rangeOf("1")})), false},
		{"invalid fingerprint", msg(&pb.Part{Kind: &pb.Part_FP{FP: &pb.Fingerprint{Range: rangesync.EncodeRange(rangeOf("01")), Count: 1}}}), false},
		{"busy and audit round are not checked", &pb.Message{Parts: []*pb.Part{in}, Busy: true, AuditRound: 9}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := s.CheckMessage(tc.m)
			if tc.valid && err != nil {
				t.Fatal(err)
			}
			if !tc.valid && !isInvalid(err) {
				t.Fatalf("got %v, want invalid", err)
			}
		})
	}
}
