// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage_test

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"errors"
	"io"
	"math/big"
	"sync"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	pstoremock "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

// TestSaveLoad tests the idempotence of saving and loading the postage.Service
// with all the active stamp issuers.
func TestSaveLoad(t *testing.T) {
	t.Parallel()

	store := inmemstore.New()
	defer store.Close()
	pstore := pstoremock.New()
	saved := func(id int64) postage.Service {
		ps, err := postage.NewService(log.Noop, store, pstore, id, false)
		if err != nil {
			t.Fatal(err)
		}
		for range 16 {
			err := ps.Add(newTestStampIssuer(t, 1000))
			if err != nil {
				t.Fatal(err)
			}
		}
		if err := ps.Close(); err != nil {
			t.Fatal(err)
		}
		return ps
	}
	loaded := func(id int64) postage.Service {
		ps, err := postage.NewService(log.Noop, store, pstore, id, false)
		if err != nil {
			t.Fatal(err)
		}
		return ps
	}
	test := func(id int64) {
		psS := saved(id)
		psL := loaded(id)
		defer psL.Close()

		sMap := map[string]struct{}{}
		stampIssuers := psS.StampIssuers()
		for _, s := range stampIssuers {
			sMap[string(s.ID())] = struct{}{}
		}

		stampIssuers = psL.StampIssuers()
		for _, s := range stampIssuers {
			if _, ok := sMap[string(s.ID())]; !ok {
				t.Fatalf("mismatch between saved and loaded")
			}
		}
	}
	test(0)
	test(1)
}

func TestGetStampIssuer(t *testing.T) {
	t.Parallel()

	store := inmemstore.New()
	defer store.Close()
	chainID := int64(0)
	testChainState := postagetesting.NewChainState()
	if testChainState.Block < uint64(postage.BlockThreshold) {
		testChainState.Block += uint64(postage.BlockThreshold + 1)
	}
	validBlockNumber := testChainState.Block - uint64(postage.BlockThreshold+1)
	pstore := pstoremock.New(pstoremock.WithChainState(testChainState))
	ps, err := postage.NewService(log.Noop, store, pstore, chainID, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close()

	ids := make([][]byte, 8)
	for i := range ids {
		id := make([]byte, 32)
		_, err := io.ReadFull(crand.Reader, id)
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = id
		if i == 0 {
			continue
		}

		var shift uint64 = 0
		if i > 3 {
			shift = uint64(i)
		}
		err = ps.Add(postage.NewStampIssuer(
			string(id),
			"",
			id,
			big.NewInt(3),
			16,
			8,
			validBlockNumber+shift, true),
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Run("found", func(t *testing.T) {
		for _, id := range ids[1:4] {
			st, save, err := ps.GetStampIssuer(id)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			_ = save()
			if st.Label() != string(id) {
				t.Fatalf("wrong issuer returned")
			}
		}

		// check if the save() call persisted the stamp issuers
		for _, id := range ids[1:4] {
			stampIssuerItem := postage.NewStampIssuerItem(id)
			err := store.Get(stampIssuerItem)
			if err != nil {
				t.Fatal(err)
			}
			if string(id) != stampIssuerItem.ID() {
				t.Fatalf("got id %s, want id %s", stampIssuerItem.ID(), string(id))
			}
		}
	})
	t.Run("not found", func(t *testing.T) {
		_, _, err := ps.GetStampIssuer(ids[0])
		if !errors.Is(err, postage.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
	t.Run("not usable", func(t *testing.T) {
		for _, id := range ids[4:] {
			_, _, err := ps.GetStampIssuer(id)
			if !errors.Is(err, postage.ErrNotUsable) {
				t.Fatalf("expected ErrNotUsable, got %v", err)
			}
		}
	})
	t.Run("recovered", func(t *testing.T) {
		b := postagetesting.MustNewBatch()
		b.Start = validBlockNumber
		testAmount := big.NewInt(1)
		err := ps.HandleCreate(b, testAmount)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		st, sv, err := ps.GetStampIssuer(b.ID)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if st.Label() != "recovered" {
			t.Fatal("wrong issuer returned")
		}
		err = sv()
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("topup", func(t *testing.T) {
		ps.HandleTopUp(ids[1], big.NewInt(10))
		if err != nil {
			t.Fatal(err)
		}
		stampIssuer, save, err := ps.GetStampIssuer(ids[1])
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		_ = save()
		if stampIssuer.Amount().Cmp(big.NewInt(13)) != 0 {
			t.Fatalf("expected amount %d got %d", 13, stampIssuer.Amount().Int64())
		}
	})
	t.Run("dilute", func(t *testing.T) {
		ps.HandleDepthIncrease(ids[2], 17)
		if err != nil {
			t.Fatal(err)
		}
		stampIssuer, save, err := ps.GetStampIssuer(ids[2])
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		_ = save()
		if stampIssuer.Amount().Cmp(big.NewInt(3)) != 0 {
			t.Fatalf("expected amount %d got %d", 3, stampIssuer.Amount().Int64())
		}
		if stampIssuer.Depth() != 17 {
			t.Fatalf("expected depth %d got %d", 17, stampIssuer.Depth())
		}
	})
}

func TestSetExpired(t *testing.T) {
	t.Parallel()

	store := inmemstore.New()
	testutil.CleanupCloser(t, store)

	batch := swarm.RandAddress(t).Bytes()
	notExistsBatch := swarm.RandAddress(t).Bytes()

	pstore := pstoremock.New(pstoremock.WithExistsFunc(func(b []byte) (bool, error) {
		return bytes.Equal(b, batch), nil
	}))

	ps, err := postage.NewService(log.Noop, store, pstore, 1, false)
	if err != nil {
		t.Fatal(err)
	}

	itemExists := postage.NewStampItem().WithChunkAddress(swarm.RandAddress(t)).WithBatchID(batch)
	err = store.Put(itemExists)
	if err != nil {
		t.Fatal(err)
	}

	itemNotExists := postage.NewStampItem().WithChunkAddress(swarm.RandAddress(t)).WithBatchID(notExistsBatch)
	err = store.Put(itemNotExists)
	if err != nil {
		t.Fatal(err)
	}

	err = ps.Add(newTestStampIssuerID(t, 1000, itemExists.BatchID))
	if err != nil {
		t.Fatal(err)
	}
	err = ps.Add(newTestStampIssuerID(t, 1000, itemNotExists.BatchID))
	if err != nil {
		t.Fatal(err)
	}
	err = ps.HandleStampExpiry(context.Background(), itemNotExists.BatchID)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = ps.GetStampIssuer(itemNotExists.BatchID)
	if !errors.Is(err, postage.ErrNotFound) {
		t.Fatalf("expected %v, got %v", postage.ErrNotFound, err)
	}

	err = store.Iterate(
		storage.Query{
			Factory: func() storage.Item {
				return new(postage.StampItem)
			},
		}, func(result storage.Result) (bool, error) {
			item := result.Entry.(*postage.StampItem)
			exists, err := pstore.Exists(item.BatchID)
			if err != nil {
				return false, err
			}

			if bytes.Equal(item.BatchID, notExistsBatch) && exists {
				return false, errors.New("found stamp item belonging to a non-existent batch")
			}

			if bytes.Equal(item.BatchID, batch) && !exists {
				return false, errors.New("found stamp item belonging to a batch that should exist")
			}

			return false, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	err = store.Get(itemExists)
	if err != nil {
		t.Fatal(err)
	}

	err = store.Get(itemNotExists)
	if err == nil {
		t.Fatal("expected error getting expired stamp item, got nil")
	}

	testutil.CleanupCloser(t, ps)
}

// TestCrashRecovery verifies that bucket counts are restored from stamp items
// when the service starts after an unclean shutdown (wasDirty=true).
func TestCrashRecovery(t *testing.T) {
	t.Parallel()

	store := inmemstore.New()
	defer store.Close()
	pstore := pstoremock.New()

	issuer := newTestStampIssuer(t, 1000)
	batchID := issuer.ID()

	// Pick two random chunk addresses and compute their bucket indices.
	chunkAddr0 := swarm.RandAddress(t)
	chunkAddr1 := swarm.RandAddress(t)
	bIdx0 := postage.ToBucket(issuer.BucketDepth(), chunkAddr0)
	bIdx1 := postage.ToBucket(issuer.BucketDepth(), chunkAddr1)

	// Write StampItems directly, simulating stamps issued before a crash
	// without the issuer bucket state being saved.
	// bIdx0: issued at collision count 2 → bucket should recover to 3
	// bIdx1: issued at collision count 0 → bucket should recover to 1
	items := []*postage.StampItem{
		postage.NewStampItem().WithBatchID(batchID).WithChunkAddress(chunkAddr0).WithBatchIndex(postage.IndexToBytes(bIdx0, 2)),
		postage.NewStampItem().WithBatchID(batchID).WithChunkAddress(chunkAddr1).WithBatchIndex(postage.IndexToBytes(bIdx1, 0)),
	}
	for _, item := range items {
		if err := store.Put(item); err != nil {
			t.Fatal(err)
		}
	}

	// Save the issuer with zero bucket counts to the store.
	ps, err := postage.NewService(log.Noop, store, pstore, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.Add(issuer); err != nil {
		t.Fatal(err)
	}
	if err := ps.Close(); err != nil {
		t.Fatal(err)
	}

	// Verify that the issuer on disk still has zero bucket counts (i.e., recovery
	// has not happened yet). Open with wasDirty=false to skip recovery.
	psCheck, err := postage.NewService(log.Noop, store, pstore, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	checkIssuers := psCheck.StampIssuers()
	if len(checkIssuers) != 1 {
		t.Fatalf("pre-recovery check: expected 1 issuer, got %d", len(checkIssuers))
	}
	checkBuckets := checkIssuers[0].Buckets()
	if checkBuckets[bIdx0] != 0 {
		t.Errorf("pre-recovery check: bucket %d: want 0, got %d", bIdx0, checkBuckets[bIdx0])
	}
	if checkBuckets[bIdx1] != 0 {
		t.Errorf("pre-recovery check: bucket %d: want 0, got %d", bIdx1, checkBuckets[bIdx1])
	}
	if err := psCheck.Close(); err != nil {
		t.Fatal(err)
	}

	// Restart with wasDirty=true — should trigger bucket recovery.
	ps2, err := postage.NewService(log.Noop, store, pstore, 1, true)
	if err != nil {
		t.Fatal(err)
	}

	issuers := ps2.StampIssuers()
	if len(issuers) != 1 {
		t.Fatalf("expected 1 issuer, got %d", len(issuers))
	}

	buckets := issuers[0].Buckets()
	if buckets[bIdx0] != 3 {
		t.Errorf("bucket %d: want 3, got %d", bIdx0, buckets[bIdx0])
	}
	if buckets[bIdx1] != 1 {
		t.Errorf("bucket %d: want 1, got %d", bIdx1, buckets[bIdx1])
	}

	// Clean shutdown — recovered counts must be flushed to disk.
	if err := ps2.Close(); err != nil {
		t.Fatal(err)
	}

	// Restart with wasDirty=false — recovery is skipped, but counts must still
	// be correct because ps2.Close() persisted the recovered state.
	ps3, err := postage.NewService(log.Noop, store, pstore, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ps3.Close()

	issuers3 := ps3.StampIssuers()
	if len(issuers3) != 1 {
		t.Fatalf("post-recovery clean restart: expected 1 issuer, got %d", len(issuers3))
	}

	buckets3 := issuers3[0].Buckets()
	if buckets3[bIdx0] != 3 {
		t.Errorf("post-recovery clean restart: bucket %d: want 3, got %d", bIdx0, buckets3[bIdx0])
	}
	if buckets3[bIdx1] != 1 {
		t.Errorf("post-recovery clean restart: bucket %d: want 1, got %d", bIdx1, buckets3[bIdx1])
	}
}

func TestUpdateIssuerLabel(t *testing.T) {
	t.Parallel()

	store := inmemstore.New()
	defer store.Close()
	pstore := pstoremock.New()

	ps, err := postage.NewService(log.Noop, store, pstore, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close()

	issuer := newTestStampIssuer(t, 1000)
	batchID := issuer.ID()

	if err := ps.Add(issuer); err != nil {
		t.Fatal(err)
	}

	t.Run("not found", func(t *testing.T) {
		unknown := make([]byte, 32)
		if err := ps.UpdateIssuerLabel(unknown, "x"); !errors.Is(err, postage.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("updates in-memory and persists", func(t *testing.T) {
		const newLabel = "updated-label"

		if err := ps.UpdateIssuerLabel(batchID, newLabel); err != nil {
			t.Fatal(err)
		}

		// in-memory label must reflect the update immediately
		issuers := ps.StampIssuers()
		var found bool
		for _, si := range issuers {
			if bytes.Equal(si.ID(), batchID) {
				found = true
				if si.Label() != newLabel {
					t.Fatalf("in-memory label: got %q, want %q", si.Label(), newLabel)
				}
			}
		}
		if !found {
			t.Fatal("issuer not found after update")
		}

		// persisted label must also reflect the update
		item := postage.NewStampIssuerItem(batchID)
		if err := store.Get(item); err != nil {
			t.Fatal(err)
		}
		if item.Issuer.Label() != newLabel {
			t.Fatalf("persisted label: got %q, want %q", item.Issuer.Label(), newLabel)
		}
	})
}

func TestResetStampIssuer(t *testing.T) {
	t.Parallel()

	store := inmemstore.New()
	pstore := pstoremock.New(pstoremock.WithChainState(&postage.ChainState{Block: 1000}))
	ps, err := postage.NewService(log.Noop, store, pstore, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, ps)

	batchID := make([]byte, 32)
	batchID[0] = 42

	mutableIssuer := postage.NewStampIssuer("label", "keyID", batchID, big.NewInt(3), 16, 8, 0, false)
	if err := ps.Add(mutableIssuer); err != nil {
		t.Fatal(err)
	}

	addr := swarm.NewAddress([]byte{1, 2, 3, 4})
	for range mutableIssuer.BucketUpperBound() {
		if _, _, err := mutableIssuer.Increment(addr); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := mutableIssuer.Increment(addr); !errors.Is(err, postage.ErrBucketFull) {
		t.Fatalf("want %v, got %v", postage.ErrBucketFull, err)
	}

	if err := ps.ResetStampIssuer(batchID); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if got := mutableIssuer.Generation(); got != 1 {
		t.Fatalf("generation: want 1, got %d", got)
	}
	if got := mutableIssuer.Utilization(); got != 0 {
		t.Fatalf("utilization: want 0, got %d", got)
	}
	if _, _, err := mutableIssuer.Increment(addr); err != nil {
		t.Fatalf("increment after reset: %v", err)
	}

	// The reset is persisted immediately, not on the next periodic save.
	item := &postage.StampIssuerItem{Issuer: postage.NewStampIssuer("", "", batchID, nil, 0, 0, 0, false)}
	if err := store.Get(item); err != nil {
		t.Fatal(err)
	}
	if got := item.Issuer.Generation(); got != 1 {
		t.Fatalf("persisted generation: want 1, got %d", got)
	}

	notFoundID := make([]byte, 32)
	notFoundID[0] = 99
	if err := ps.ResetStampIssuer(notFoundID); !errors.Is(err, postage.ErrNotFound) {
		t.Fatalf("want %v, got %v", postage.ErrNotFound, err)
	}

	immutableBatchID := make([]byte, 32)
	immutableBatchID[0] = 77
	immutableIssuer := postage.NewStampIssuer("label", "keyID", immutableBatchID, big.NewInt(3), 16, 8, 0, true)
	if err := ps.Add(immutableIssuer); err != nil {
		t.Fatal(err)
	}
	if err := ps.ResetStampIssuer(immutableBatchID); !errors.Is(err, postage.ErrBatchImmutable) {
		t.Fatalf("want %v, got %v", postage.ErrBatchImmutable, err)
	}
	if got := immutableIssuer.Generation(); got != 0 {
		t.Fatalf("immutable generation: want 0, got %d", got)
	}
}

// resetTestSetup creates a service with a mutable issuer whose buckets hold
// two chunks each, and returns three addresses that fall into bucket 0.
func resetTestSetup(t *testing.T, store storage.Store) (postage.Service, *postage.StampIssuer, crypto.Signer, [3]swarm.Address) {
	t.Helper()

	pstore := pstoremock.New(pstoremock.WithChainState(&postage.ChainState{Block: 1000}))
	ps, err := postage.NewService(log.Noop, store, pstore, 1, false)
	if err != nil {
		t.Fatal(err)
	}

	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	batchID := make([]byte, 32)
	batchID[0] = 42
	// batch depth 16, bucket depth 15: each bucket holds two chunks.
	issuer := postage.NewStampIssuer("label", "keyID", batchID, big.NewInt(3), 16, 15, 0, false)
	if err := ps.Add(issuer); err != nil {
		t.Fatal(err)
	}

	return ps, issuer, crypto.NewDefaultSigner(privKey), [3]swarm.Address{
		swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000001"),
		swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000002"),
		swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000003"),
	}
}

func mustStamp(t *testing.T, stamper postage.Stamper, addr swarm.Address) []byte {
	t.Helper()
	st, err := stamper.Stamp(addr, addr)
	if err != nil {
		t.Fatalf("stamp %s: %v", addr, err)
	}
	return st.Index()
}

// TestResetStampIssuerReindexesOldChunks checks that after a reset a chunk
// stamped before the reset does not reuse its old batch index, which may by
// then be assigned to a different chunk.
func TestResetStampIssuerReindexesOldChunks(t *testing.T) {
	t.Parallel()

	store := inmemstore.New()
	ps, issuer, signer, addrs := resetTestSetup(t, store)
	testutil.CleanupCloser(t, ps)
	a, b, c := addrs[0], addrs[1], addrs[2]

	stamper := postage.NewStamper(store, issuer, signer)
	mustStamp(t, stamper, a)
	mustStamp(t, stamper, c)
	if _, err := stamper.Stamp(b, b); !errors.Is(err, postage.ErrBucketFull) {
		t.Fatalf("want %v, got %v", postage.ErrBucketFull, err)
	}

	if err := ps.ResetStampIssuer(issuer.ID()); err != nil {
		t.Fatal(err)
	}

	stamper = postage.NewStamper(store, issuer, signer)
	idxB := mustStamp(t, stamper, b)
	idxA := mustStamp(t, stamper, a)
	if bytes.Equal(idxA, idxB) {
		t.Fatalf("chunk stamped before reset reused index %x already assigned after reset", idxA)
	}
	// Re-stamping within the same generation keeps the index.
	if idx := mustStamp(t, stamper, a); !bytes.Equal(idx, idxA) {
		t.Fatalf("re-stamp in same generation: want index %x, got %x", idxA, idx)
	}
}

// TestResetStampIssuerInvalidatesStampers checks that a stamper created
// before a reset, as held by an in-flight upload, stops issuing stamps
// instead of reusing indices it has already issued.
func TestResetStampIssuerInvalidatesStampers(t *testing.T) {
	t.Parallel()

	store := inmemstore.New()
	ps, issuer, signer, addrs := resetTestSetup(t, store)
	testutil.CleanupCloser(t, ps)

	stamper := postage.NewStamper(store, issuer, signer)
	mustStamp(t, stamper, addrs[0])

	if err := ps.ResetStampIssuer(issuer.ID()); err != nil {
		t.Fatal(err)
	}

	for _, addr := range addrs {
		if _, err := stamper.Stamp(addr, addr); !errors.Is(err, postage.ErrStampIssuerReset) {
			t.Fatalf("stamp %s: want %v, got %v", addr, postage.ErrStampIssuerReset, err)
		}
	}
	mustStamp(t, postage.NewStamper(store, issuer, signer), addrs[1])
}

// TestResetStampIssuerCrashRecovery checks that crash recovery after a reset
// restores the bucket counts from stamps of the current generation only.
// Counting stale stamps would mark the bucket full again.
func TestResetStampIssuerCrashRecovery(t *testing.T) {
	t.Parallel()

	store := inmemstore.New()
	ps, issuer, signer, addrs := resetTestSetup(t, store)
	// Closed only after the assertions, as closing persists the issuer.
	testutil.CleanupCloser(t, ps)
	bucket := postage.ToBucket(issuer.BucketDepth(), addrs[0])

	stamper := postage.NewStamper(store, issuer, signer)
	mustStamp(t, stamper, addrs[0])
	mustStamp(t, stamper, addrs[1])

	if err := ps.ResetStampIssuer(issuer.ID()); err != nil {
		t.Fatal(err)
	}
	mustStamp(t, postage.NewStamper(store, issuer, signer), addrs[2])

	// Simulate an unclean shutdown: the issuer was last persisted by the
	// reset, so the stamp issued afterwards is only recorded as a stamp item.
	pstore := pstoremock.New(pstoremock.WithChainState(&postage.ChainState{Block: 1000}))
	ps2, err := postage.NewService(log.Noop, store, pstore, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, ps2)

	issuers := ps2.StampIssuers()
	if len(issuers) != 1 {
		t.Fatalf("want 1 issuer, got %d", len(issuers))
	}
	recovered := issuers[0]
	if got := recovered.Generation(); got != 1 {
		t.Fatalf("generation: want 1, got %d", got)
	}
	if got := recovered.Buckets()[bucket]; got != 1 {
		t.Fatalf("bucket %d: want 1, got %d", bucket, got)
	}

	// The re-opened issuer still treats pre-reset stamps as stale.
	stamper = postage.NewStamper(store, recovered, signer)
	idxA := mustStamp(t, stamper, addrs[0])
	idxC := mustStamp(t, stamper, addrs[2])
	if bytes.Equal(idxA, idxC) {
		t.Fatalf("pre-reset chunk reused index %x of a post-reset chunk", idxA)
	}
}

// TestResetStampIssuerConcurrentStamping runs resets concurrently with
// stamping to exercise the locking between the service and its stampers.
func TestResetStampIssuerConcurrentStamping(t *testing.T) {
	t.Parallel()

	store := inmemstore.New()
	ps, issuer, signer, _ := resetTestSetup(t, store)
	testutil.CleanupCloser(t, ps)

	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for range 4 {
		wg.Go(func() {
			stamper := postage.NewStamper(store, issuer, signer)
			for range 200 {
				addr := swarm.RandAddress(t)
				_, err := stamper.Stamp(addr, addr)
				switch {
				case err == nil, errors.Is(err, postage.ErrBucketFull):
				case errors.Is(err, postage.ErrStampIssuerReset):
					stamper = postage.NewStamper(store, issuer, signer)
				default:
					errs <- err
					return
				}
			}
		})
	}
	for range 20 {
		if err := ps.ResetStampIssuer(issuer.ID()); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("stamp: %v", err)
	}
	if got := issuer.Generation(); got != 20 {
		t.Fatalf("generation: want 20, got %d", got)
	}
}
