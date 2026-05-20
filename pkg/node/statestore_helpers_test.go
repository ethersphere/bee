// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node_test

import (
	"strings"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/node"
	statestoremock "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// These tests cover the small persistence helpers in pkg/node/statestore.go.
// They are pure logic over a StateStorer and therefore safe to unit-test with
// the mock store; the actual leveldb backing is exercised by InitStateStore /
// InitStamperStore tests in their own files.

func TestOverlayNonceExists(t *testing.T) {
	t.Parallel()

	t.Run("not present returns false and a zero-filled nonce", func(t *testing.T) {
		t.Parallel()
		s := statestoremock.NewStateStore()
		nonce, exists, err := node.OverlayNonceExists(s)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if exists {
			t.Fatal("expected exists=false on a fresh store")
		}
		if len(nonce) != 32 {
			t.Fatalf("expected a 32-byte nonce buffer, got %d", len(nonce))
		}
	})

	t.Run("present returns the stored bytes and true", func(t *testing.T) {
		t.Parallel()
		s := statestoremock.NewStateStore()
		want := make([]byte, 32)
		for i := range want {
			want[i] = byte(i + 1)
		}
		if err := s.Put(node.OverlayNonceKey, want); err != nil {
			t.Fatalf("put nonce: %v", err)
		}
		nonce, exists, err := node.OverlayNonceExists(s)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !exists {
			t.Fatal("expected exists=true when nonce is stored")
		}
		if string(nonce) != string(want) {
			t.Fatalf("nonce mismatch: got % x, want % x", nonce, want)
		}
	})
}

func TestSetOverlay_RoundtripsBothKeys(t *testing.T) {
	t.Parallel()

	s := statestoremock.NewStateStore()
	overlay := swarm.RandAddress(t)
	nonce := make([]byte, 32)
	for i := range nonce {
		nonce[i] = byte(i)
	}

	if err := node.SetOverlay(s, overlay, nonce); err != nil {
		t.Fatalf("SetOverlay: %v", err)
	}

	var gotNonce []byte
	if err := s.Get(node.OverlayNonceKey, &gotNonce); err != nil {
		t.Fatalf("Get nonce: %v", err)
	}
	if string(gotNonce) != string(nonce) {
		t.Fatalf("stored nonce mismatch: got % x, want % x", gotNonce, nonce)
	}

	var gotOverlay swarm.Address
	if err := s.Get(node.NoncedOverlayKey, &gotOverlay); err != nil {
		t.Fatalf("Get overlay: %v", err)
	}
	if !gotOverlay.Equal(overlay) {
		t.Fatalf("stored overlay mismatch: got %s, want %s", gotOverlay, overlay)
	}
}

func TestCheckOverlay(t *testing.T) {
	t.Parallel()

	t.Run("empty store writes the overlay and returns nil", func(t *testing.T) {
		t.Parallel()
		s := statestoremock.NewStateStore()
		overlay := swarm.RandAddress(t)

		if err := node.CheckOverlay(s, overlay); err != nil {
			t.Fatalf("first call expected nil, got %v", err)
		}

		var stored swarm.Address
		if err := s.Get(node.NoncedOverlayKey, &stored); err != nil {
			t.Fatalf("expected overlay to have been written: %v", err)
		}
		if !stored.Equal(overlay) {
			t.Fatalf("stored overlay mismatch: got %s, want %s", stored, overlay)
		}
	})

	t.Run("matching stored overlay returns nil", func(t *testing.T) {
		t.Parallel()
		s := statestoremock.NewStateStore()
		overlay := swarm.RandAddress(t)
		if err := s.Put(node.NoncedOverlayKey, overlay); err != nil {
			t.Fatalf("put overlay: %v", err)
		}
		if err := node.CheckOverlay(s, overlay); err != nil {
			t.Fatalf("expected nil when stored overlay matches, got %v", err)
		}
	})

	t.Run("differing stored overlay returns error", func(t *testing.T) {
		t.Parallel()
		s := statestoremock.NewStateStore()
		stored := swarm.RandAddress(t)
		incoming := swarm.RandAddress(t)
		if stored.Equal(incoming) {
			t.Skip("random addresses collided; rerun")
		}
		if err := s.Put(node.NoncedOverlayKey, stored); err != nil {
			t.Fatalf("put overlay: %v", err)
		}
		err := node.CheckOverlay(s, incoming)
		if err == nil {
			t.Fatal("expected error when stored overlay differs")
		}
		if !strings.Contains(err.Error(), "overlay address changed") {
			t.Fatalf("expected error to mention overlay change, got %q", err.Error())
		}
	})
}
