// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullsync_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/ethersphere/bee/v2/pkg/bitvector"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/postage"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/pullsync"
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
// stamp fields have the wrong length: like any other malformed offer, it fails
// the interval and nothing is stored.
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
			ps, _              = newPullSync(t, nil, 5, mock.WithSubscribeResp(tResults, nil), mock.WithChunks(chunks...))
			recorder           = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
			psClient, clientDb = newPullSync(t, recorder, 0)
		)

		topmost, count, err := psClient.Sync(context.Background(), swarm.ZeroAddress, 0, 0)
		if err == nil || !strings.Contains(err.Error(), "inconsistent stamp length") {
			t.Fatalf("got error %v, want inconsistent stamp length", err)
		}
		if topmost != 0 {
			t.Fatalf("got topmost %d, want 0", topmost)
		}
		if count != 0 {
			t.Fatalf("got count %d, want 0", count)
		}
		if p := clientDb.PutCalls(); p != 0 {
			t.Fatalf("want 0 puts but got %d", p)
		}
	})
}

// Offers encoded by the released 1.4.0 protocol (bee v2.8.3) for one chunk with
// address 0x01.., batch ID 0x02.. and stamp hash 0x03.., topmost 7. The first
// was produced by the v2.8.3 encoder; the second is the same offer with a sum
// of 0x07.., which the v2.8.3 decoder reads back with all fields intact.
const (
	releasedOfferHex = "080712660a20" + "0101010101010101010101010101010101010101010101010101010101010101" +
		"1220" + "0202020202020202020202020202020202020202020202020202020202020202" +
		"1a20" + "0303030303030303030303030303030303030303030303030303030303030303"
	offerWithSumHex = "080712780a20" + "0101010101010101010101010101010101010101010101010101010101010101" +
		"1220" + "0202020202020202020202020202020202020202020202020202020202020202" +
		"1a20" + "0303030303030303030303030303030303030303030303030303030303030303" +
		"2210" + "07070707070707070707070707070707"
)

// TestWireCompat checks the offer encoding against bytes from the released
// 1.4.0 protocol: without a sum the offer is byte for byte the released one,
// and with a sum the only addition is field 4 appended to the chunk, which a
// released decoder skips as an unknown field.
func TestWireCompat(t *testing.T) {
	t.Parallel()

	chunk := &pb.Chunk{
		Address:   bytes.Repeat([]byte{0x01}, swarm.HashSize),
		BatchID:   bytes.Repeat([]byte{0x02}, swarm.HashSize),
		StampHash: bytes.Repeat([]byte{0x03}, swarm.HashSize),
	}

	for _, tc := range []struct {
		name string
		sum  []byte
		want string
	}{
		{"without sum", nil, releasedOfferHex},
		{"with sum", bytes.Repeat([]byte{0x07}, storage.ChunkSumSize), offerWithSumHex},
	} {
		c := *chunk
		c.Sum = tc.sum
		got, err := (&pb.Offer{Topmost: 7, Chunks: []*pb.Chunk{&c}}).Marshal()
		if err != nil {
			t.Fatal(err)
		}
		if h := hex.EncodeToString(got); h != tc.want {
			t.Fatalf("%s: got %s, want %s", tc.name, h, tc.want)
		}
	}
}

// TestLegacyInterop syncs between nodes on real reserves where one side
// behaves like a released 1.4.0 node, in both directions: a serving node whose
// offers carry no sum, and a puller that decides on batch ID and stamp hash
// only.
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
			oldServer := newSyncerOn(t, nil, legacyOffers{dbOld})
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
			toNew := streamtest.New(streamtest.WithProtocols(newServer.Protocol()))
			if n := legacyPull(t, toNew, dbOld, bin); n != 1 {
				t.Fatalf("legacy from new: put %d, want 1", n)
			}
			if n := legacyPull(t, toNew, dbOld, bin); n != 0 {
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

		oldServer := newSyncerOn(t, nil, legacyOffers{dbOld})
		aFromOld := newReserveSyncer(t, streamtest.New(streamtest.WithProtocols(oldServer.Protocol())), dbA)
		// the legacy link cannot see the divergence: same stamp, no want,
		// no error and no retry loop.
		if n := syncFrom(t, aFromOld, bin); n != 0 {
			t.Fatalf("legacy link put %d, want 0", n)
		}
		expectReserveEntry(t, dbA, stamped(loser, shared), stamped(winner, shared))

		// the legacy node pulling from A does not want it either.
		aServer := newReserveSyncer(t, nil, dbA)
		if n := legacyPull(t, streamtest.New(streamtest.WithProtocols(aServer.Protocol())), dbOld, bin); n != 0 {
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

// legacyOffers serves a real reserve the way a released 1.4.0 node does: its
// offers carry batch ID and stamp hash but no sum, which encodes byte for byte
// like a released offer (see TestWireCompat).
type legacyOffers struct {
	*storer.DB
}

func (l legacyOffers) SubscribeBin(ctx context.Context, bin uint8, start uint64) (<-chan *storer.BinC, func(), <-chan error) {
	in, unsub, errC := l.DB.SubscribeBin(ctx, bin, start)
	out := make(chan *storer.BinC)
	quit := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(out)
		for c := range in {
			cp := *c
			cp.Sum = nil
			select {
			case out <- &cp:
			case <-quit:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, func() {
		once.Do(func() { close(quit) })
		unsub()
	}, errC
}

func newSyncerOn(t *testing.T, recorder *streamtest.Recorder, store storer.Reserve) *pullsync.Syncer {
	t.Helper()

	validStamp := func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }
	ps := pullsync.New(recorder, store, func(swarm.Chunk) {}, func(*soc.SOC) {}, validStamp, log.Noop, 10)
	t.Cleanup(func() {
		if err := ps.Close(); err != nil {
			t.Errorf("close pullsync: %v", err)
		}
	})
	return ps
}

// legacyPull runs one pull of a bin the way a released 1.4.0 puller does: it
// wants the offered chunks it lacks by address, batch ID and stamp hash,
// ignoring any sum, and accepts a delivery only for a wanted stamp. It returns
// the number of chunks put.
func legacyPull(t *testing.T, streamer *streamtest.Recorder, db *storer.DB, bin uint8) int {
	t.Helper()
	ctx := context.Background()

	stream, err := streamer.NewStream(ctx, swarm.ZeroAddress, nil, pullsync.ProtocolName, pullsync.ProtocolVersion, pullsync.StreamName)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stream.FullClose() }()
	w, r := protobuf.NewWriterAndReader(stream)

	if err := w.WriteMsgWithContext(ctx, &pb.Get{Bin: int32(bin)}); err != nil {
		t.Fatal(err)
	}
	var offer pb.Offer
	if err := r.ReadMsgWithContext(ctx, &offer); err != nil {
		t.Fatal(err)
	}
	if len(offer.Chunks) == 0 {
		return 0
	}

	bv, err := bitvector.New(len(offer.Chunks))
	if err != nil {
		t.Fatal(err)
	}
	wanted := make(map[string]struct{})
	for i, c := range offer.Chunks {
		a := swarm.NewAddress(c.Address)
		has, err := db.ReserveHasStamp(a, c.BatchID, c.StampHash)
		if err != nil {
			t.Fatal(err)
		}
		if !has {
			wanted[a.ByteString()+string(c.BatchID)+string(c.StampHash)] = struct{}{}
			bv.Set(i)
		}
	}
	if err := w.WriteMsgWithContext(ctx, &pb.Want{BitVector: bv.Bytes()}); err != nil {
		t.Fatal(err)
	}

	put := 0
	for n := len(wanted); n > 0; n-- {
		var d pb.Delivery
		if err := r.ReadMsgWithContext(ctx, &d); err != nil {
			t.Fatal(err)
		}
		stamp := new(postage.Stamp)
		if err := stamp.UnmarshalBinary(d.Stamp); err != nil {
			t.Fatal(err)
		}
		stampHash, err := stamp.Hash()
		if err != nil {
			t.Fatal(err)
		}
		addr := swarm.NewAddress(d.Address)
		if _, ok := wanted[addr.ByteString()+string(stamp.BatchID())+string(stampHash)]; !ok {
			t.Fatalf("unsolicited delivery of %s", addr)
		}
		if err := db.ReservePutter().Put(ctx, swarm.NewChunk(addr, d.Data).WithStamp(stamp)); err != nil {
			t.Fatal(err)
		}
		put++
	}
	return put
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
