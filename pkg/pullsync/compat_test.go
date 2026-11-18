// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullsync_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/pullsync/internal/legacy"
	legacypb "github.com/ethersphere/bee/v2/pkg/pullsync/internal/legacy/pb"
	"github.com/ethersphere/bee/v2/pkg/pullsync/pb"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	mock "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestIncoming_LegacyOffer syncs an offer in which some chunks carry no sum,
// as a peer predating SWIP-101 sends them. Those are wanted by stamp, the rest
// by sum, and both kinds of delivery are accepted in one stream. A second
// round wants nothing.
func TestIncoming_LegacyOffer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tResults := make([]*storer.BinC, len(results))
		for i, r := range results {
			cp := *r
			if i%2 == 0 {
				cp.Sum = nil
			}
			tResults[i] = &cp
		}

		var (
			// one response per round
			ps, _              = newPullSync(t, nil, 5, mock.WithSubscribeResp(tResults, nil), mock.WithSubscribeResp(tResults, nil), mock.WithChunks(chunks...))
			recorder           = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
			psClient, clientDb = newPullSync(t, recorder, 0)
		)

		_, count, err := psClient.Sync(context.Background(), swarm.ZeroAddress, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if count != len(chunks) {
			t.Fatalf("got count %d, want %d", count, len(chunks))
		}
		haveChunks(t, clientDb, chunks...)

		_, count, err = psClient.Sync(context.Background(), swarm.ZeroAddress, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("second round got count %d, want 0", count)
		}
	})
}

// TestIncoming_LegacyOfferStampLength covers an offer without a sum whose
// stamp fields have the wrong length: the entry is skipped and reported, the
// rest of the offer is stored and the cursor still advances.
func TestIncoming_LegacyOfferStampLength(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tResults := make([]*storer.BinC, len(results))
		for i, r := range results {
			cp := *r
			cp.Sum = nil
			tResults[i] = &cp
		}
		tResults[2].BatchID = tResults[2].BatchID[:storage.ChunkSumSize]

		var (
			topMost            = uint64(4)
			ps, _              = newPullSync(t, nil, 5, mock.WithSubscribeResp(tResults, nil), mock.WithChunks(chunks...))
			recorder           = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
			psClient, clientDb = newPullSync(t, recorder, 0)
		)

		topmost, count, err := psClient.Sync(context.Background(), swarm.ZeroAddress, 0, 0)
		if err == nil || !strings.Contains(err.Error(), "inconsistent stamp length") {
			t.Fatalf("got error %v, want inconsistent stamp length", err)
		}
		if topmost != topMost {
			t.Fatalf("got topmost %d, want %d", topmost, topMost)
		}
		if count != len(chunks)-1 {
			t.Fatalf("got count %d, want %d", count, len(chunks)-1)
		}
		haveChunks(t, clientDb, append(chunks[:2:2], chunks[3:]...)...)
		if has, _ := clientDb.ReserveHas(chunks[2].Address(), results[2].Sum); has {
			t.Fatal("chunk with malformed offer stamp must not be stored")
		}
	})
}

// TestWireCompat checks the offer encoding against the 1.4.0 one on master:
// without a sum it is byte for byte the legacy encoding, and a legacy decoder
// reads an offer with a sum by skipping the unknown field.
func TestWireCompat(t *testing.T) {
	t.Parallel()

	addr, batchID, stampHash := swarm.RandAddress(t).Bytes(), swarm.RandAddress(t).Bytes(), swarm.RandAddress(t).Bytes()
	sum := bytes.Repeat([]byte{7}, storage.ChunkSumSize)

	legacyBytes, err := (&legacypb.Offer{Topmost: 9, Chunks: []*legacypb.Chunk{{Address: addr, BatchID: batchID, StampHash: stampHash}}}).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	noSum, err := (&pb.Offer{Topmost: 9, Chunks: []*pb.Chunk{{Address: addr, BatchID: batchID, StampHash: stampHash}}}).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacyBytes, noSum) {
		t.Fatal("offer without sum differs from the legacy encoding")
	}

	withSum, err := (&pb.Offer{Topmost: 9, Chunks: []*pb.Chunk{{Address: addr, BatchID: batchID, StampHash: stampHash, Sum: sum}}}).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	var got legacypb.Offer
	if err := got.Unmarshal(withSum); err != nil {
		t.Fatalf("legacy decode: %v", err)
	}
	c := got.Chunks[0]
	if got.Topmost != 9 || !bytes.Equal(c.Address, addr) || !bytes.Equal(c.BatchID, batchID) || !bytes.Equal(c.StampHash, stampHash) {
		t.Fatal("legacy decode lost fields")
	}
}

// TestLegacyInterop syncs between a node running this protocol and a node
// running the 1.4.0 protocol from master, in both directions, over real
// reserves.
func TestLegacyInterop(t *testing.T) {
	t.Parallel()

	signer := crypto.NewDefaultSigner(mustKey(t))
	id := make([]byte, swarm.HashSize)
	newSOC := func(payload string) swarm.Chunk {
		t.Helper()
		ch, err := cac.New([]byte(payload))
		if err != nil {
			t.Fatal(err)
		}
		sch, err := soc.New(id, ch).Sign(signer)
		if err != nil {
			t.Fatal(err)
		}
		return sch
	}

	t.Run("plain chunks both directions", func(t *testing.T) {
		t.Parallel()

		base := swarm.RandAddress(t)
		batch := postagetesting.MustNewBatch()
		cacCh := stamped(mustCAC(t, "cac"), postagetesting.MustNewFields(batch.ID, 1, 1))
		socCh := stamped(newSOC("soc"), postagetesting.MustNewFields(batch.ID, 2, 1))

		for _, ch := range []swarm.Chunk{cacCh, socCh} {
			bin := swarm.Proximity(base.Bytes(), ch.Address().Bytes())

			// new pulls from legacy
			dbNew, dbOld := newReserveDB(t, base), newReserveDB(t, base)
			putReserve(t, dbOld, ch)
			oldServer := newLegacySyncer(t, nil, dbOld)
			newClient := newReserveSyncer(t, streamtest.New(streamtest.WithProtocols(oldServer.Protocol())), dbNew)
			if n := syncFrom(t, newClient, bin); n != 1 {
				t.Fatalf("new from legacy: put %d, want 1", n)
			}
			if n := syncFrom(t, newClient, bin); n != 0 {
				t.Fatalf("new from legacy second round: put %d, want 0", n)
			}

			// legacy pulls from new
			dbNew, dbOld = newReserveDB(t, base), newReserveDB(t, base)
			putReserve(t, dbNew, ch)
			newServer := newReserveSyncer(t, nil, dbNew)
			oldClient := newLegacySyncer(t, streamtest.New(streamtest.WithProtocols(newServer.Protocol())), dbOld)
			if n := legacySyncFrom(t, oldClient, bin); n != 1 {
				t.Fatalf("legacy from new: put %d, want 1", n)
			}
			if n := legacySyncFrom(t, oldClient, bin); n != 0 {
				t.Fatalf("legacy from new second round: put %d, want 0", n)
			}
		}
	})

	t.Run("mixed neighborhood heals through upgraded peers", func(t *testing.T) {
		t.Parallel()

		v1, v2 := newSOC("payload one"), newSOC("payload two")
		winner, loser := v2, v1
		if wins, err := storage.ReplaceIncoming(v2, v1); err != nil {
			t.Fatal(err)
		} else if wins {
			winner, loser = v1, v2
		}
		shared := postagetesting.MustNewFields(postagetesting.MustNewBatch().ID, 0, 1)

		base := swarm.RandAddress(t)
		bin := swarm.Proximity(base.Bytes(), v1.Address().Bytes())
		dbA, dbOld, dbC := newReserveDB(t, base), newReserveDB(t, base), newReserveDB(t, base)
		putReserve(t, dbA, stamped(loser, shared))    // upgraded node, losing payload
		putReserve(t, dbOld, stamped(winner, shared)) // legacy node
		putReserve(t, dbC, stamped(winner, shared))   // upgraded node

		oldServer := newLegacySyncer(t, nil, dbOld)
		aFromOld := newReserveSyncer(t, streamtest.New(streamtest.WithProtocols(oldServer.Protocol())), dbA)
		// the legacy link cannot see the divergence: same stamp, no want,
		// no error and no retry loop.
		if n := syncFrom(t, aFromOld, bin); n != 0 {
			t.Fatalf("legacy link put %d, want 0", n)
		}
		expectReserveEntry(t, dbA, stamped(loser, shared), stamped(winner, shared))

		// the legacy node pulling from A does not want it either.
		aServer := newReserveSyncer(t, nil, dbA)
		oldFromA := newLegacySyncer(t, streamtest.New(streamtest.WithProtocols(aServer.Protocol())), dbOld)
		if n := legacySyncFrom(t, oldFromA, bin); n != 0 {
			t.Fatalf("legacy client put %d, want 0", n)
		}

		// an upgraded peer offers the sum and A converges.
		cServer := newReserveSyncer(t, nil, dbC)
		aFromC := newReserveSyncer(t, streamtest.New(streamtest.WithProtocols(cServer.Protocol())), dbA)
		if n := syncFrom(t, aFromC, bin); n != 1 {
			t.Fatalf("upgraded link put %d, want 1", n)
		}
		expectReserveEntry(t, dbA, stamped(winner, shared), stamped(loser, shared))
	})
}

func newLegacySyncer(t *testing.T, recorder *streamtest.Recorder, db *storer.DB) *legacy.Syncer {
	t.Helper()

	validStamp := func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }
	ps := legacy.New(recorder, db, func(swarm.Chunk) {}, func(*soc.SOC) {}, validStamp, log.Noop, 10)
	t.Cleanup(func() {
		if err := ps.Close(); err != nil {
			t.Errorf("close legacy pullsync: %v", err)
		}
	})
	return ps
}

func legacySyncFrom(t *testing.T, client *legacy.Syncer, bin uint8) int {
	t.Helper()
	_, count, err := client.Sync(context.Background(), swarm.ZeroAddress, bin, 0)
	if err != nil {
		t.Fatalf("legacy sync: %v", err)
	}
	return count
}

func mustKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	k, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func mustCAC(t *testing.T, payload string) swarm.Chunk {
	t.Helper()
	ch, err := cac.New([]byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	return ch
}
