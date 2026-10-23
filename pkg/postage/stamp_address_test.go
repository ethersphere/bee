// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestStampValidRejectsMalformedChunkAddress checks that chunk addresses that
// are not HashSize long are rejected before toBucket reads their first four
// bytes.
func TestStampValidRejectsMalformedChunkAddress(t *testing.T) {
	t.Parallel()

	for _, length := range []int{0, 1, 3, 31, 33} {
		t.Run(fmt.Sprintf("%d-bytes", length), func(t *testing.T) {
			t.Parallel()

			addr := swarm.NewAddress(make([]byte, length))
			stamp := signedStampFor(t, addr)

			err := stamp.Valid(addr, make([]byte, 20), 16, 8, false)
			if !errors.Is(err, postage.ErrInvalidChunkAddress) {
				t.Fatalf("got error %v, want %v", err, postage.ErrInvalidChunkAddress)
			}
		})
	}
}

// signedStampFor builds a stamp whose signature recovers for addr, so that
// validation would proceed past RecoverBatchOwner.
func signedStampFor(t *testing.T, addr swarm.Address) *postage.Stamp {
	t.Helper()

	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	batchID := make([]byte, 32)
	index := make([]byte, 8)
	timestamp := make([]byte, 8)

	toSign, err := postage.ToSignDigest(addr.Bytes(), batchID, index, timestamp)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := crypto.NewDefaultSigner(key).Sign(toSign)
	if err != nil {
		t.Fatal(err)
	}
	return postage.NewStamp(batchID, index, timestamp, sig)
}
