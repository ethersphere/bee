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
	"testing"

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
		ps, err := postage.NewService(log.Noop, store, pstore, id, true)
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
		ps, err := postage.NewService(log.Noop, store, pstore, id, true)
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
	ps, err := postage.NewService(log.Noop, store, pstore, chainID, true)
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

	ps, err := postage.NewService(log.Noop, store, pstore, 1, true)
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
		t.Fatal(err)
	}

	testutil.CleanupCloser(t, ps)
}

// TestCrashRecovery verifies that bucket counts are restored from stamp items
// when the service starts after an unclean shutdown (wasClean=false).
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
	ps, err := postage.NewService(log.Noop, store, pstore, 1, true)
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
	// has not happened yet). Open with wasClean=true to skip recovery.
	psCheck, err := postage.NewService(log.Noop, store, pstore, 1, true)
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

	// Restart with wasClean=false — should trigger bucket recovery.
	ps2, err := postage.NewService(log.Noop, store, pstore, 1, false)
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

	// Restart with wasClean=true — recovery is skipped, but counts must still
	// be correct because ps2.Close() persisted the recovered state.
	ps3, err := postage.NewService(log.Noop, store, pstore, 1, true)
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
