// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package storage_test

import (
	"encoding/hex"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestIdentityAddress(t *testing.T) {
	t.Run("single owner chunk", func(t *testing.T) {
		t.Parallel()
		// Create a single owner chunk (SOC)
		owner := common.HexToAddress("8d3766440f0d7b949a5e32995d09619a7f86e632")
		// signature of hash(id + chunk address of foo)
		sig, err := hex.DecodeString("5acd384febc133b7b245e5ddc62d82d2cded9182d2716126cd8844509af65a053deb418208027f548e3e88343af6f84a8772fb3cebc0a1833a0ea7ec0c1348311b")
		if err != nil {
			t.Fatal(err)
		}
		id := make([]byte, swarm.HashSize)
		copy(id, []byte("id"))
		payload := []byte("foo")
		ch, err := cac.New(payload)
		if err != nil {
			t.Fatal(err)
		}
		sch, err := soc.NewSigned(id, ch, owner.Bytes(), sig)
		if err != nil {
			t.Fatal(err)
		}
		schChunk, err := sch.Chunk()
		if err != nil {
			t.Fatal(err)
		}
		schAddress, err := sch.Address()
		if err != nil {
			t.Fatal(err)
		}

		idAddr, err := storage.IdentityAddress(schChunk)
		if err != nil {
			t.Fatalf("IdentityAddress returned error: %v", err)
		}

		if idAddr.IsZero() {
			t.Fatalf("expected non-zero address, got zero address")
		}

		if idAddr.Equal(schAddress) {
			t.Fatalf("expected identity address to be different from SOC address")
		}
	})

	t.Run("content addressed chunk", func(t *testing.T) {
		t.Parallel()
		// Create a content addressed chunk (CAC)
		data := []byte("data")
		cacChunk, err := cac.New(data)
		if err != nil {
			t.Fatalf("failed to create content addressed chunk: %v", err)
		}

		// Call IdentityAddress with the CAC
		addr, err := storage.IdentityAddress(cacChunk)
		if err != nil {
			t.Fatalf("IdentityAddress returned error: %v", err)
		}

		// Verify the address matches the CAC address
		if !addr.Equal(cacChunk.Address()) {
			t.Fatalf("expected address %s, got %s", cacChunk.Address(), addr)
		}
	})
}
