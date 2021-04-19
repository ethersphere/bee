// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage_test

import (
	crand "crypto/rand"
	"errors"
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
	createStamp := func(t *testing.T, stamper postage.Stamper) (swarm.Address, *postage.Stamp) {
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
		if err := stamp.Valid(chunkAddr, owner, 8, 12, true); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	// tests that Stamps returns with postage.ErrBucketMismatch
	t.Run("bucket mismatch", func(t *testing.T) {
		st := newTestStampIssuer(t)
		stamper := postage.NewStamper(st, signer)
		chunkAddr, stamp := createStamp(t, stamper)
		a := chunkAddr.Bytes()
		a[0] ^= 0xff
		if err := stamp.Valid(swarm.NewAddress(a), owner, 8, 12, true); !errors.Is(err, postage.ErrBucketMismatch) {
			t.Fatalf("expected ErrBucketMismatch, got %v", err)
		}
	})

	// tests that Stamps returns with postage.ErrInvalidIndex
	t.Run("invalid index", func(t *testing.T) {
		st := newTestStampIssuer(t)
		stamper := postage.NewStamper(st, signer)
		// issue 1 stamp
		chunkAddr, _ := createStamp(t, stamper)
		// issue another 15
		// collision depth is 8, committed batch depth is 12, bucket volume 2^4
		for i := 0; i < 14; i++ {
			_, err = stamper.Stamp(chunkAddr)
			if err != nil {
				t.Fatalf("error adding stamp at step %d: %v", i, err)
			}
		}
		stamp, err := stamper.Stamp(chunkAddr)
		if err != nil {
			t.Fatalf("error adding last stamp: %v", err)
		}
		if err := stamp.Valid(chunkAddr, owner, 8, 11, true); !errors.Is(err, postage.ErrInvalidIndex) {
			t.Fatalf("expected ErrInvalidIndex, got %v", err)
		}
	})

	// tests that Stamps returns with postage.ErrBucketFull iff
	// issuer has the corresponding collision bucket filled]
	t.Run("bucket full", func(t *testing.T) {
		st := newTestStampIssuer(t)
		st = postage.NewStampIssuer("", "", st.ID(), 12, 8)
		stamper := postage.NewStamper(st, signer)
		// issue 1 stamp
		chunkAddr, _ := createStamp(t, stamper)
		// issue another 15
		// collision depth is 8, committed batch depth is 12, bucket volume 2^4
		for i := 0; i < 15; i++ {
			_, err = stamper.Stamp(chunkAddr)
			if err != nil {
				t.Fatalf("error adding stamp at step %d: %v", i, err)
			}
		}
		// the bucket should now be full, not allowing a stamp for the  pivot chunk
		if _, err = stamper.Stamp(chunkAddr); !errors.Is(err, postage.ErrBucketFull) {
			t.Fatalf("expected ErrBucketFull, got %v", err)
		}
	})

	// tests return with ErrOwnerMismatch
	t.Run("owner mismatch", func(t *testing.T) {
		owner[0] ^= 0xff // bitflip the owner first byte, this case must come last!
		st := newTestStampIssuer(t)
		stamper := postage.NewStamper(st, signer)
		chunkAddr, stamp := createStamp(t, stamper)
		if err := stamp.Valid(chunkAddr, owner, 8, 12, true); !errors.Is(err, postage.ErrOwnerMismatch) {
			t.Fatalf("expected ErrOwnerMismatch, got %v", err)
		}
	})

}
