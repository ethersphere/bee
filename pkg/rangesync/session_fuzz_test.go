// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"errors"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
)

// FuzzSessionHandle feeds a responder or an initiator two arbitrary messages.
// Handle must not panic; its errors are violations or ErrPeerBusy; every
// message it returns is within the limits and passes the peer's stateless
// checks.
func FuzzSessionHandle(f *testing.F) {
	rng := newRand(11)
	// the root is 2 bits deep and the leaf depth 6, so one split reaches it
	scope := rangesync.Scope{Root: rangeOf("01"), LeafDepth: 6}
	ids := randIDs(rng, scope.Root, 300)
	local := newFakeLocal(ids...)
	slicesSort(ids)

	enc := func(m *pb.Message) []byte {
		data, err := m.Marshal()
		if err != nil {
			f.Fatal(err)
		}
		return data
	}
	children := make([]*pb.Part, 0, 1<<fingerprint.FanoutBits)
	for _, c := range scope.Root.Children(fingerprint.FanoutBits) {
		children = append(children, fpPart(c, sum(3, 1)))
	}
	opening := enc(msg(fpPart(scope.Root, sum(5, 5))))
	f.Add(false, opening, enc(msg()))
	f.Add(false, opening, enc(msg(itemsPart(rangesync.Items{Range: scope.Root.Children(4)[3], IDs: ids[:2], Reply: true}))))
	f.Add(false, enc(msg(fpPart(rangeOf("0110"), sum(0, 0)), fpPart(rangeOf("0111"), sum(1, 2)))), enc(msg()))
	f.Add(true, enc(msg(children...)), enc(msg()))
	f.Add(true, enc(msg(itemsPart(rangesync.Items{Range: scope.Root, IDs: ids[:3], More: true, Reply: true}))),
		enc(msg(itemsPart(rangesync.Items{Range: scope.Root, IDs: ids[3:5], After: idPtr(ids[2]), Reply: true}))))
	f.Add(true, enc(&pb.Message{Busy: true}), []byte{})
	f.Add(true, []byte{}, []byte{})

	f.Fuzz(func(t *testing.T, initiator bool, first, second []byte) {
		cfg := rangesync.Config{Scope: scope, Local: local}
		var s *rangesync.Session
		// ranges the peer sent Fingerprints or Reply pages for: the only
		// ranges this side may split or list
		asked := map[fingerprint.RangeKey]bool{}
		if initiator {
			var err error
			s, _, err = rangesync.NewInitiator(cfg)
			if err != nil {
				t.Fatal(err)
			}
		} else {
			s = rangesync.NewResponder(cfg)
		}
		listed := map[fingerprint.ID]bool{}
		wanted := map[fingerprint.ID]bool{}
		for _, data := range [][]byte{first, second} {
			in := new(pb.Message)
			if err := in.Unmarshal(data); err != nil {
				return
			}
			out, wants, err := s.Handle(in)
			if err != nil {
				if !isInvalid(err) && !errors.Is(err, rangesync.ErrPeerBusy) {
					t.Fatalf("unexpected error %v", err)
				}
				return
			}
			// the message was valid, so it parses
			fps, items := partsOf(t, in)
			for _, r := range fps {
				asked[r.Key()] = true
			}
			for _, it := range items {
				for _, id := range it.IDs {
					listed[id] = true
				}
				// a page without Reply asks for nothing, but an Items
				// answer may cover it when it answers our own Reply page
				asked[it.Range.Key()] = asked[it.Range.Key()] || it.Reply
			}
			for _, id := range wants {
				if !listed[id] || wanted[id] {
					t.Fatalf("want %x not listed by the peer, or wanted twice", id[:4])
				}
				wanted[id] = true
				if has, _ := local.HasID(id); has {
					t.Fatalf("want %x is held", id[:4])
				}
			}
			if out == nil {
				return
			}
			got := wire(t, out)
			if err := scope.CheckMessage(got); err != nil {
				t.Fatalf("returned message fails the peer's checks: %v", err)
			}
			fps, items = partsOf(t, got)
			for _, r := range fps {
				parent := fingerprint.NewRange(r.Prefix, r.Bits-fingerprint.FanoutBits)
				if r.Bits < fingerprint.FanoutBits || !asked[parent.Key()] {
					t.Fatalf("fingerprint %s splits a range the peer did not send", r)
				}
			}
			for _, it := range items {
				if !asked[it.Range.Key()] {
					t.Fatalf("items for %s, which the peer did not ask about", it.Range)
				}
				for _, id := range it.IDs {
					if has, _ := local.HasID(id); !has {
						t.Fatalf("sent id %x that is not held", id[:4])
					}
				}
			}
		}
	})
}

// FuzzSessionConverges drives two honest sessions over fuzzed sets and
// scopes and checks that each side wants exactly what the other holds and it
// lacks inside the session root. The seeds include a root at the leaf depth
// with 40000 common IDs, whose Items streams span three or more messages.
func FuzzSessionConverges(f *testing.F) {
	f.Add(uint64(1), uint16(40000), uint8(3), uint8(3), uint8(0), uint8(0), false)
	f.Add(uint64(2), uint16(3000), uint8(50), uint8(50), uint8(12), uint8(1), false)
	f.Add(uint64(3), uint16(2000), uint8(20), uint8(20), uint8(8), uint8(2), true)
	f.Add(uint64(4), uint16(0), uint8(200), uint8(0), uint8(12), uint8(0), false)
	f.Add(uint64(5), uint16(20000), uint8(255), uint8(255), uint8(3), uint8(1), false)

	f.Fuzz(func(t *testing.T, seed uint64, common uint16, onlyA, onlyB, leafBits, rootDelta uint8, paired bool) {
		rng := newRand(seed)
		localRoot := rangeOf("01")
		remoteRoot := randRange(rng, localRoot, int(rootDelta%3))
		scope, err := rangesync.NewScope(status(localRoot, fingerprint.LeafBits), status(remoteRoot, leafBits%(fingerprint.LeafBits+1)))
		if err != nil {
			return
		}
		la, lb, a2b, b2a := convergenceSets(rng, localRoot, scope, int(common)%40001, int(onlyA), int(onlyB), paired)
		a, open, err := rangesync.NewInitiator(rangesync.Config{Scope: scope, Local: la})
		if err != nil {
			t.Fatal(err)
		}
		b := rangesync.NewResponder(rangesync.Config{Scope: scope, Local: lb})
		wantsA, wantsB, _ := drive(t, a, open, b, pageChecker(t, new(int)))
		assertSet(t, wantsA, a2b)
		assertSet(t, wantsB, b2a)
	})
}

// randRange returns a random sub-range of r that is extra bits deeper.
func randRange(rng *rand.Rand, r fingerprint.Range, extra int) fingerprint.Range {
	id := randID(rng, r)
	return fingerprint.NewRange(id.Address(), r.Bits+uint8(extra))
}

// convergenceSets returns two locals sharing common IDs in gen, with onlyA
// and onlyB IDs on one side only, and the IDs inside the scope root that A
// lacks (a2b) and B lacks (b2a). With paired, every A-only ID shares its
// leaf range (at the scope's leaf depth) with a B-only one, so counts are
// equal at every level.
func convergenceSets(rng *rand.Rand, gen fingerprint.Range, scope rangesync.Scope, common, onlyA, onlyB int, paired bool) (la, lb *fakeLocal, a2b, b2a []fingerprint.ID) {
	shared := randIDs(rng, gen, common)
	var as, bs []fingerprint.ID
	if paired {
		for range min(onlyA, onlyB) {
			leaf := randRange(rng, scope.Root, int(scope.LeafDepth-scope.Root.Bits))
			as = append(as, randID(rng, leaf))
			bs = append(bs, randID(rng, leaf))
		}
	} else {
		as = randIDs(rng, gen, onlyA)
		bs = randIDs(rng, gen, onlyB)
	}
	for _, id := range as {
		if scope.Root.ContainsID(id) {
			b2a = append(b2a, id)
		}
	}
	for _, id := range bs {
		if scope.Root.ContainsID(id) {
			a2b = append(a2b, id)
		}
	}
	return newFakeLocal(append(slices.Clone(shared), as...)...), newFakeLocal(append(slices.Clone(shared), bs...)...), a2b, b2a
}
