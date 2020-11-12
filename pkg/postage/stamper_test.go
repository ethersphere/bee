// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage_test

import (
	crand "crypto/rand"
	"io"
	"testing"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestStamperStamping tests if the stamp created by the stamper is valid.
func TestStamperStamping(t *testing.T) {
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	owner, err := crypto.NewEthereumAddress(privKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)
	createStamp := func(t *testing.T, stamper *postage.Stamper) (swarm.Address, *postage.Stamp) {
		t.Helper()
		h := make([]byte, 32)
		_, err = io.ReadFull(crand.Reader, h)
		if err != nil {
			t.Fatal(err)
		}
		chunkAddr := swarm.NewAddress(h)
		stamp, err := stamper.Stamp(chunkAddr)
		if err != nil {
			t.Fatal(err)
		}
		return chunkAddr, stamp
	}

	// tests a valid stamp
	t.Run("valid stamp", func(t *testing.T) {
		st := newTestStampIssuer(t)
		stamper := postage.NewStamper(st, signer)
		chunkAddr, stamp := createStamp(t, stamper)
		if err := stamp.Valid(chunkAddr, owner); err != nil {
			t.Fatal(err)
		}
	})

	// invalid stamp, incorrect chunk address (it still returns postage.ErrOwnerMismatch)
	t.Run("invalid stamp", func(t *testing.T) {
		st := newTestStampIssuer(t)
		stamper := postage.NewStamper(st, signer)
		chunkAddr, stamp := createStamp(t, stamper)
		a := chunkAddr.Bytes()
		a[0] ^= 0xff
		if err := stamp.Valid(swarm.NewAddress(a), owner); err != postage.ErrOwnerMismatch {
			t.Fatalf("expected ErrOwnerMismatch, got %v", err)
		}
	})

	// tests that Stamps returns with postage.ErrBucketFull iff
	// issuer has the corresponding collision bucket filled]
	t.Run("bucket full", func(t *testing.T) {
		b := newTestBatch(t, owner)
		st := postage.NewStampIssuer("", "", b.ID, b.Depth, 8)
		stamper := postage.NewStamper(st, signer)
		// issue 1 stamp
		chunkAddr, _ := createStamp(t, stamper)
		// issue another 255
		// collision depth is 8, committed batch depth is 16, bucket volume 2^8
		for i := 0; i < 255; i++ {
			h := make([]byte, 32)
			_, err = io.ReadFull(crand.Reader, h)
			if err != nil {
				t.Fatal(err)
			}
			// generate a chunks matching on the first 8 bits,
			// i.e., fall into the same collision bucket
			h[0] = chunkAddr.Bytes()[0]
			// calling Inc we pretend a stamp was issued to the address
			err = st.Inc(swarm.NewAddress(h))
			if err != nil {
				t.Fatal(err)
			}
		}
		// the bucket should now be full, not allowing a stamp for the  pivot chunk
		_, err := stamper.Stamp(chunkAddr)
		if err != postage.ErrBucketFull {
			t.Fatalf("expected ErrBucketFull, got %v", err)
		}
	})

	// tests return with ErrOwnerMismatch
	t.Run("owner mismatch", func(t *testing.T) {
		owner[0] ^= 0xff // bitflip the owner first byte, this case must come last!
		st := newTestStampIssuer(t)
		stamper := postage.NewStamper(st, signer)
		chunkAddr, stamp := createStamp(t, stamper)
		if err := stamp.Valid(chunkAddr, owner); err != postage.ErrOwnerMismatch {
			t.Fatalf("expected ErrOwnerMismatch, got %v", err)
		}
	})

}
