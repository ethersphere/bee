// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullsync_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	batchstore "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/pullsync"
	"github.com/ethersphere/bee/v2/pkg/soc"
	stabilmock "github.com/ethersphere/bee/v2/pkg/stabilization/mock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	kademlia "github.com/ethersphere/bee/v2/pkg/topology/mock"
)

// TestDivergentSOCConvergesBetweenReserves syncs two nodes backed by real
// reserves that hold different payloads of one single owner chunk, in both
// directions, and checks that they settle on the same payload and sums. The
// mock reserve cannot represent divergence, so this covers the interplay of
// the offered sums, the want decision and the reserve's retention rules.
func TestDivergentSOCConvergesBetweenReserves(t *testing.T) {
	t.Parallel()

	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	owner := crypto.NewDefaultSigner(privKey)
	id := make([]byte, swarm.HashSize)
	newSOC := func(payload string) swarm.Chunk {
		t.Helper()
		ch, err := cac.New([]byte(payload))
		if err != nil {
			t.Fatal(err)
		}
		sch, err := soc.New(id, ch).Sign(owner)
		if err != nil {
			t.Fatal(err)
		}
		return sch
	}

	v1, v2 := newSOC("payload one"), newSOC("payload two")
	sameStampWinner, sameStampLoser := v2, v1
	if wins, err := storage.DivergentSocChunkWins(v2, v1); err != nil {
		t.Fatal(err)
	} else if wins {
		sameStampWinner, sameStampLoser = v1, v2
	}

	batchA := postagetesting.MustNewBatch()
	batchB := postagetesting.MustNewBatch()
	shared := postagetesting.MustNewFields(batchA.ID, 0, 1)
	older := postagetesting.MustNewFields(batchA.ID, 0, 1)
	newer := postagetesting.MustNewFields(batchB.ID, 0, 2)

	for _, tc := range []struct {
		name string
		// held by node a and node b before syncing
		a, b swarm.Chunk
		// stamps both nodes must end up holding, all serving winner
		stamps        []swarm.Stamp
		winner, loser swarm.Chunk
	}{
		{
			// one stamp on two payloads: the content tie-break decides
			name:   "same stamp",
			a:      stamped(v1, shared),
			b:      stamped(v2, shared),
			stamps: []swarm.Stamp{shared},
			winner: sameStampWinner,
			loser:  sameStampLoser,
		},
		{
			// stamps of different batches: the higher timestamp decides and
			// both stamps are kept against the winning payload
			name:   "different stamps",
			a:      stamped(v1, older),
			b:      stamped(v2, newer),
			stamps: []swarm.Stamp{older, newer},
			winner: v2,
			loser:  v1,
		},
	} {
		for _, aPullsFirst := range []bool{true, false} {
			name := tc.name + ", b pulls first"
			if aPullsFirst {
				name = tc.name + ", a pulls first"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				base := swarm.RandAddress(t)
				bin := swarm.Proximity(base.Bytes(), tc.a.Address().Bytes())
				dbA, dbB := newReserveDB(t, base), newReserveDB(t, base)
				putReserve(t, dbA, tc.a)
				putReserve(t, dbB, tc.b)

				psA, psB := newReserveSyncer(t, nil, dbA), newReserveSyncer(t, nil, dbB)
				// each client streams to the other node's protocol
				bFromA := newReserveSyncer(t, streamtest.New(streamtest.WithProtocols(psA.Protocol())), dbB)
				aFromB := newReserveSyncer(t, streamtest.New(streamtest.WithProtocols(psB.Protocol())), dbA)
				first, second := bFromA, aFromB
				if aPullsFirst {
					first, second = aFromB, bFromA
				}

				syncFrom(t, first, bin)
				syncFrom(t, second, bin)

				for _, db := range []*storer.DB{dbA, dbB} {
					if got, want := db.ReserveSize(), len(tc.stamps); got != want {
						t.Fatalf("reserve size %d, want %d", got, want)
					}
					for _, stamp := range tc.stamps {
						expectReserveEntry(t, db, stamped(tc.winner, stamp), stamped(tc.loser, stamp))
					}
				}

				// each node now holds every (address, sum) the other offers,
				// so a second round wants nothing.
				for _, client := range []*pullsync.Syncer{bFromA, aFromB} {
					if n := syncFrom(t, client, bin); n != 0 {
						t.Fatalf("second round put %d chunks, want 0", n)
					}
				}
			})
		}
	}
}

// stamped returns a copy of ch carrying stamp, leaving ch untouched.
func stamped(ch swarm.Chunk, stamp swarm.Stamp) swarm.Chunk {
	return swarm.NewChunk(ch.Address(), ch.Data()).WithStamp(stamp)
}

func newReserveDB(t *testing.T, base swarm.Address) *storer.DB {
	t.Helper()

	db, err := storer.New(context.Background(), "", &storer.Options{
		Logger:                log.Noop,
		Address:               base,
		CacheCapacity:         1 << 10,
		ReserveCapacity:       1 << 10,
		ReserveWakeUpDuration: time.Hour,
		RadiusSetter:          kademlia.NewTopologyDriver(),
		Batchstore:            batchstore.New(),
		StartupStabilizer:     stabilmock.NewSubscriber(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close storer: %v", err)
		}
	})
	return db
}

func newReserveSyncer(t *testing.T, recorder *streamtest.Recorder, db *storer.DB) *pullsync.Syncer {
	t.Helper()

	validStamp := func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }
	ps := pullsync.New(recorder, db, func(swarm.Chunk) {}, func(*soc.SOC) {}, validStamp, log.Noop, 10)
	t.Cleanup(func() {
		if err := ps.Close(); err != nil {
			t.Errorf("close pullsync: %v", err)
		}
	})
	return ps
}

func putReserve(t *testing.T, db *storer.DB, ch swarm.Chunk) {
	t.Helper()
	if err := db.ReservePutter().Put(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
}

// syncFrom runs one sync of the bin and returns the number of chunks put.
func syncFrom(t *testing.T, client *pullsync.Syncer, bin uint8) int {
	t.Helper()
	_, count, err := client.Sync(context.Background(), swarm.ZeroAddress, bin, 0)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	return count
}

// expectReserveEntry checks that the entry for the stamp of want serves the
// payload of want and that only its sum, not the sum of lost, is indexed.
func expectReserveEntry(t *testing.T, db *storer.DB, want, lost swarm.Chunk) {
	t.Helper()

	stampHash, err := want.Stamp().Hash()
	if err != nil {
		t.Fatal(err)
	}
	got, err := db.ReserveGet(context.Background(), want.Address(), want.Stamp().BatchID(), stampHash)
	if err != nil {
		t.Fatalf("reserve get: %v", err)
	}
	if !bytes.Equal(got.Data(), want.Data()) {
		t.Fatal("entry serves the losing payload")
	}

	for _, tc := range []struct {
		ch  swarm.Chunk
		has bool
	}{{want, true}, {lost, false}} {
		sum, err := storage.ChunkSum(tc.ch)
		if err != nil {
			t.Fatal(err)
		}
		has, err := db.ReserveHas(want.Address(), sum)
		if err != nil {
			t.Fatal(err)
		}
		if has != tc.has {
			t.Fatalf("sum of %s payload indexed: %v, want %v", map[bool]string{true: "winning", false: "losing"}[tc.has], has, tc.has)
		}
	}
}
