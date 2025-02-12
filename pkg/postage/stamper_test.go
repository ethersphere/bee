// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage_test

import (
	"bytes"
	"errors"
	"math/big"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestStamperStamping tests if the stamp created by the stamper is valid.
func TestStamperStamping(t *testing.T) {
	t.Parallel()

	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	owner, err := crypto.NewEthereumAddress(privKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)
	createStamp := func(t *testing.T, stamper postage.Stamper) (swarm.Address, *postage.Stamp) {
		t.Helper()

		chunkAddr := swarm.RandAddress(t)
		stamp, err := stamper.Stamp(chunkAddr, chunkAddr)
		if err != nil {
			t.Fatal(err)
		}
		return chunkAddr, stamp
	}

	// tests a valid stamp
	t.Run("valid stamp", func(t *testing.T) {
		st := newTestStampIssuer(t, 1000)
		stamper := postage.NewStamper(inmemstore.New(), st, signer)
		chunkAddr, stamp := createStamp(t, stamper)
		if err := stamp.Valid(chunkAddr, owner, 12, 8, true); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	// tests that Stamps returns with postage.ErrBucketMismatch
	t.Run("bucket mismatch", func(t *testing.T) {
		st := newTestStampIssuer(t, 1000)
		stamper := postage.NewStamper(inmemstore.New(), st, signer)
		chunkAddr, stamp := createStamp(t, stamper)
		a := chunkAddr.Bytes()
		a[0] ^= 0xff
		if err := stamp.Valid(swarm.NewAddress(a), owner, 12, 8, true); !errors.Is(err, postage.ErrBucketMismatch) {
			t.Fatalf("expected ErrBucketMismatch, got %v", err)
		}
	})

	// tests that Stamps returns with postage.ErrInvalidIndex
	t.Run("invalid index", func(t *testing.T) {
		st := newTestStampIssuer(t, 1000)
		stamper := postage.NewStamper(inmemstore.New(), st, signer)
		// issue 1 stamp
		chunkAddr, _ := createStamp(t, stamper)
		// issue another 15
		// collision depth is 8, committed batch depth is 12, bucket volume 2^4
		for i := 0; i < 14; i++ {
			randAddr := swarm.RandAddressAt(t, chunkAddr, 8)
			_, err = stamper.Stamp(randAddr, randAddr)
			if err != nil {
				t.Fatalf("error adding stamp at step %d: %v", i, err)
			}
		}
		randAddr := swarm.RandAddressAt(t, chunkAddr, 8)
		stamp, err := stamper.Stamp(randAddr, randAddr)
		if err != nil {
			t.Fatalf("error adding last stamp: %v", err)
		}
		if err := stamp.Valid(chunkAddr, owner, 11, 8, true); !errors.Is(err, postage.ErrInvalidIndex) {
			t.Fatalf("expected ErrInvalidIndex, got %v", err)
		}
	})

	// tests that Stamps returns with postage.ErrBucketFull iff
	// issuer has the corresponding collision bucket filled]
	t.Run("bucket full", func(t *testing.T) {
		st := postage.NewStampIssuer("", "", newTestStampIssuer(t, 1000).ID(), big.NewInt(3), 12, 8, 1000, true)
		stamper := postage.NewStamper(inmemstore.New(), st, signer)
		// issue 1 stamp
		chunkAddr, _ := createStamp(t, stamper)
		// issue another 15
		// collision depth is 8, committed batch depth is 12, bucket volume 2^4
		for i := 0; i < 15; i++ {
			randAddr := swarm.RandAddressAt(t, chunkAddr, 8)
			_, err = stamper.Stamp(randAddr, randAddr)
			if err != nil {
				t.Fatalf("error adding stamp at step %d: %v", i, err)
			}
		}
		randAddr := swarm.RandAddressAt(t, chunkAddr, 8)
		// the bucket should now be full, not allowing a stamp for the  pivot chunk
		if _, err = stamper.Stamp(randAddr, randAddr); !errors.Is(err, postage.ErrBucketFull) {
			t.Fatalf("expected ErrBucketFull, got %v", err)
		}
	})

	t.Run("reuse index but get new timestamp for mutable or immutable batch", func(t *testing.T) {
		st := newTestStampIssuerMutability(t, 1000, false)
		chunkAddr := swarm.RandAddress(t)
		bIdx := postage.ToBucket(st.BucketDepth(), chunkAddr)
		index := postage.IndexToBytes(bIdx, 4)
		testItem := postage.NewStampItem().
			WithBatchID(st.ID()).
			WithChunkAddress(chunkAddr).
			WithBatchIndex(index)
		testSt := &testStore{Store: inmemstore.New(), stampItem: testItem}
		stamper := postage.NewStamper(testSt, st, signer)
		stamp, err := stamper.Stamp(chunkAddr, chunkAddr)
		if err != nil {
			t.Fatal(err)
		}
		for _, mutability := range []bool{true, false} {
			if err := stamp.Valid(chunkAddr, owner, 12, 8, mutability); err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if bytes.Equal(stamp.Timestamp(), testItem.BatchTimestamp) {
				t.Fatalf("expected timestamp to be different, got %x", stamp.Index())
			}
			if !bytes.Equal(stamp.Index(), testItem.BatchIndex) {
				t.Fatalf("expected index to be the same, got %x", stamp.Index())
			}
		}
	})

	// tests return with ErrOwnerMismatch
	t.Run("owner mismatch", func(t *testing.T) {
		owner[0] ^= 0xff // bitflip the owner first byte, this case must come last!
		st := newTestStampIssuer(t, 1000)
		stamper := postage.NewStamper(inmemstore.New(), st, signer)
		chunkAddr, stamp := createStamp(t, stamper)
		if err := stamp.Valid(chunkAddr, owner, 12, 8, true); !errors.Is(err, postage.ErrOwnerMismatch) {
			t.Fatalf("expected ErrOwnerMismatch, got %v", err)
		}
	})
}

type testStore struct {
	storage.Store
	stampItem *postage.StampItem
}

func (t *testStore) Get(item storage.Item) error {
	if item.Namespace() == (postage.StampItem{}).Namespace() {
		if t.stampItem == nil {
			return storage.ErrNotFound
		}
		*item.(*postage.StampItem) = *t.stampItem
		return nil
	}
	return t.Store.Get(item)
}
