// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"errors"
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
		if initiator {
			var err error
			s, _, err = rangesync.NewInitiator(cfg)
			if err != nil {
				t.Fatal(err)
			}
		} else {
			s = rangesync.NewResponder(cfg)
		}
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
			for _, id := range wants {
				if !scope.Root.ContainsID(id) {
					t.Fatal("want outside the scope")
				}
			}
			if out == nil {
				return
			}
			got := wire(t, out)
			if err := scope.CheckMessage(got); err != nil {
				t.Fatalf("returned message fails the peer's checks: %v", err)
			}
		}
	})
}
