// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"context"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/pullsync"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storer"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestCoexistence runs one node D with both protocols over one reserve
// against a pullsync-only peer P and a rangesync-only peer R (proposal §13,
// coexistence). D pulls P's chunks with pullsync; those land in D's reserve
// and, through the reserve observer, in its rangesync index. A rangesync
// session between D and R then neither wants the chunk D already got
// through pullsync nor misses P's chunks: after the fetches D and R both
// hold the union of the three reserves.
//
// P only serves: the reserve mock answers SubscribeBin with a fixed list, so
// D's pullsync handler cannot offer what D holds and P does not pull from D
// here (see the T1.13 entry of the Decisions log).
func TestCoexistence(t *testing.T) {
	t.Parallel()

	chs := validChunks(9)
	pOnly, rOnly, dOnly := chs[0:3], chs[3:6], chs[6:8]
	shared := chs[8] // held by P and R, not D
	pHolds := append(pOnly[:len(pOnly):len(pOnly)], shared)
	rHolds := append(rOnly[:len(rOnly):len(rOnly)], shared)

	// P: pullsync only
	offers := make([]*storer.BinC, len(pHolds))
	for i, ch := range pHolds {
		id := chunkIDOf(t, ch)
		offers[i] = &storer.BinC{Address: ch.Address(), BatchID: ch.Stamp().BatchID(), BinID: uint64(i + 1), StampHash: id.StampHash}
	}
	validStamp := func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }
	unwrap := func(swarm.Chunk) {}
	gsoc := func(*soc.SOC) {}
	pStore := mockstorer.NewReserve(mockstorer.WithSubscribeResp(offers, nil), mockstorer.WithChunks(pHolds...))
	p := pullsync.New(nil, pStore, unwrap, gsoc, validStamp, log.Noop, uint64(len(offers)))
	t.Cleanup(func() { _ = p.Close() })

	// R: rangesync only
	r := newTestNode(t, rHolds...)
	r.start(t)

	// D: both protocols over one reserve
	d := newTestNode(t, dOnly...)
	d.start(t)
	dPull := pullsync.New(d.rec, d.store, unwrap, gsoc, validStamp, log.Noop, uint64(len(offers)))
	t.Cleanup(func() { _ = dPull.Close() })
	d.rec.SetProtocols(p.Protocol(), r.svc.Protocol())
	r.rec.SetProtocols(d.svc.Protocol())

	// pullsync from P
	if _, _, err := dPull.Sync(context.Background(), swarm.RandAddress(t), 0, 1); err != nil {
		t.Fatal(err)
	}
	assertStored(t, d, true, idsOf(t, pHolds...)...)
	waitFor(t, "pulled chunks in the index", func() bool {
		st, err := d.svc.LocalStatus()
		return err == nil && st.Summary.Count == uint64(len(dOnly)+len(pHolds))
	})

	// rangesync with R
	for _, x := range [][2]*testNode{{d, r}, {r, d}} {
		if err := x[0].svc.SendStatus(context.Background(), x[1].addr); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "status", func() bool {
			_, ok := x[1].svc.LastStatus(x[0].addr)
			return ok
		})
	}
	wants, err := d.svc.Reconcile(context.Background(), r.addr)
	if err != nil {
		t.Fatal(err)
	}
	reconcileMessages(t, d, r.addr) // waits for R's handler
	// the shared chunk came through pullsync: rangesync does not want it
	assertChunkIDs(t, wants, idsOf(t, rOnly...)...)
	rWants := r.responderWants(d.addr)
	assertChunkIDs(t, rWants, idsOf(t, append(dOnly[:len(dOnly):len(dOnly)], pOnly...)...)...)

	if _, err := d.svc.Fetch(context.Background(), r.addr, wants); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Fetch(context.Background(), d.addr, rWants); err != nil {
		t.Fatal(err)
	}
	union := idsOf(t, chs...)
	assertStored(t, d, true, union...)
	assertStored(t, r, true, union...)
	if got := d.svc.CrossProtocolDuplicates() + r.svc.CrossProtocolDuplicates(); got != 0 {
		t.Fatalf("cross protocol duplicates: %v", got)
	}

	// D and R converged: equal roots
	waitFor(t, "equal roots", func() bool {
		sd, errD := d.svc.LocalStatus()
		sr, errR := r.svc.LocalStatus()
		return errD == nil && errR == nil && sd.Summary == sr.Summary && sd.Summary.Count == uint64(len(chs))
	})
}
