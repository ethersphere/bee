// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nbhdutil_test

import (
	"context"
	"math/rand"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/nbhdutil"
)

func TestMiner(t *testing.T) {
	t.Parallel()

	k, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	const (
		networkID uint64 = 1
	)
	prox := rand.Intn(16)

	randomAddr := swarm.RandAddress(t)
	prefix := bitStr(randomAddr.Bytes(), prox)

	addr, nonce, err := nbhdutil.MineOverlay(context.Background(), k.PublicKey, networkID, prefix)
	if err != nil {
		t.Fatal(err)
	}

	if got := swarm.Proximity(randomAddr.Bytes(), addr.Bytes()); got < uint8(prox) {
		t.Fatalf("mined overlay address has wrong proximity, got %d want %d", got, prox)
	}

	recoveredAddr, err := crypto.NewOverlayAddress(k.PublicKey, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}

	if !recoveredAddr.Equal(addr) {
		t.Fatalf("mined overlay address does not match, got %s want %s", addr, recoveredAddr)
	}
}

func bitStr(src []byte, bits int) string {

	ret := ""

	for _, b := range src {
		for i := 7; i >= 0; i-- {
			if b&(1<<i) > 0 {
				ret += "1"
			} else {
				ret += "0"
			}
			bits--
			if bits == 0 {
				return ret
			}
		}
	}

	return ret
}
