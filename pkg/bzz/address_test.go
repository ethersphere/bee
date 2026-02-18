// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bzz_test

import (
	"bytes"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/crypto"

	"github.com/multiformats/go-multiaddr"
)

func TestBzzAddress(t *testing.T) {
	t.Parallel()

	node1ma, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA")
	if err != nil {
		t.Fatal(err)
	}

	nonce := common.HexToHash("0x2").Bytes()

	privateKey1, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	overlay, err := crypto.NewOverlayAddress(privateKey1.PublicKey, 3, nonce)
	if err != nil {
		t.Fatal(err)
	}
	signer1 := crypto.NewDefaultSigner(privateKey1)

	bzzAddress, err := bzz.NewAddress(signer1, []multiaddr.Multiaddr{node1ma}, overlay, 3, nonce)
	if err != nil {
		t.Fatal(err)
	}

	bzzAddress2, err := bzz.ParseAddress(node1ma.Bytes(), overlay.Bytes(), bzzAddress.Signature, nonce, true, 3)
	if err != nil {
		t.Fatal(err)
	}

	if !bzzAddress.Equal(bzzAddress2) {
		t.Fatalf("got %s expected %s", bzzAddress2, bzzAddress)
	}

	bytes, err := bzzAddress.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	var newbzz bzz.Address
	if err := newbzz.UnmarshalJSON(bytes); err != nil {
		t.Fatal(err)
	}

	if !newbzz.Equal(bzzAddress) {
		t.Fatalf("got %s expected %s", newbzz, bzzAddress)
	}
}

func TestAreUnderlaysEqual(t *testing.T) {
	// --- Test Data Initialization ---
	addr1 := mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/8001")
	addr2 := mustNewMultiaddr(t, "/ip4/192.168.1.1/tcp/8002")
	addr3 := mustNewMultiaddr(t, "/ip6/::1/udp/9000")
	addr4 := mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/8001") // Identical to addr1

	// --- Test Cases Definition ---
	testCases := []struct {
		name string
		a    []multiaddr.Multiaddr
		b    []multiaddr.Multiaddr
		want bool
	}{
		{
			name: "two nil slices",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "one nil and one empty slice",
			a:    nil,
			b:    []multiaddr.Multiaddr{},
			want: true,
		},
		{
			name: "one empty and one nil slice",
			a:    []multiaddr.Multiaddr{},
			b:    nil,
			want: true,
		},
		{
			name: "two empty slices",
			a:    []multiaddr.Multiaddr{},
			b:    []multiaddr.Multiaddr{},
			want: true,
		},
		{
			name: "equal slices with same order",
			a:    []multiaddr.Multiaddr{addr1, addr2},
			b:    []multiaddr.Multiaddr{addr1, addr2},
			want: true,
		},
		{
			name: "equal slices with different order",
			a:    []multiaddr.Multiaddr{addr1, addr2, addr3},
			b:    []multiaddr.Multiaddr{addr3, addr1, addr2},
			want: true,
		},
		{
			name: "equal slices with identical (but not same instance) values",
			a:    []multiaddr.Multiaddr{addr1, addr2},
			b:    []multiaddr.Multiaddr{addr4, addr2},
			want: true,
		},
		{
			name: "slices with different lengths (a < b)",
			a:    []multiaddr.Multiaddr{addr1},
			b:    []multiaddr.Multiaddr{addr1, addr2},
			want: false,
		},
		{
			name: "slices with different lengths (b < a)",
			a:    []multiaddr.Multiaddr{addr1, addr2},
			b:    []multiaddr.Multiaddr{addr1},
			want: false,
		},
		{
			name: "slices with same length but different elements",
			a:    []multiaddr.Multiaddr{addr1, addr2},
			b:    []multiaddr.Multiaddr{addr1, addr3},
			want: false,
		},
		{
			name: "one slice is nil",
			a:    []multiaddr.Multiaddr{addr1},
			b:    nil,
			want: false,
		},
		{
			name: "slices with duplicates, equal",
			a:    []multiaddr.Multiaddr{addr1, addr2, addr1},
			b:    []multiaddr.Multiaddr{addr1, addr1, addr2},
			want: true,
		},
		{
			name: "slices with duplicates, not equal",
			a:    []multiaddr.Multiaddr{addr1, addr2, addr3},
			b:    []multiaddr.Multiaddr{addr1, addr1, addr2},
			want: false,
		},
		{
			name: "slices with different duplicates",
			a:    []multiaddr.Multiaddr{addr1, addr1, addr2},
			b:    []multiaddr.Multiaddr{addr1, addr2, addr2},
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := bzz.AreUnderlaysEqual(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("AreUnderlaysEqual() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseAddress(t *testing.T) {
	t.Parallel()

	const networkID uint64 = 10
	nonce := common.HexToHash("0x5").Bytes()

	// Generate test key pair and overlay address
	privateKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	overlay, err := crypto.NewOverlayAddress(privateKey.PublicKey, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}

	signer := crypto.NewDefaultSigner(privateKey)

	t.Run("single underlay - valid address", func(t *testing.T) {
		t.Parallel()

		underlay := mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/1634")

		addr, err := bzz.NewAddress(signer, []multiaddr.Multiaddr{underlay}, overlay, networkID, nonce)
		if err != nil {
			t.Fatal(err)
		}

		// Parse the address with overlay validation
		parsed, err := bzz.ParseAddress(addr.Underlays[0].Bytes(), overlay.Bytes(), addr.Signature, nonce, true, networkID)
		if err != nil {
			t.Fatalf("ParseAddress failed: %v", err)
		}

		if !parsed.Equal(addr) {
			t.Errorf("parsed address not equal to original: got %v, want %v", parsed, addr)
		}

		if !parsed.Overlay.Equal(overlay) {
			t.Errorf("overlay mismatch: got %v, want %v", parsed.Overlay, overlay)
		}

		if len(parsed.Underlays) != 1 {
			t.Errorf("expected 1 underlay, got %d", len(parsed.Underlays))
		}
	})

	t.Run("multiple underlays - valid address", func(t *testing.T) {
		t.Parallel()

		underlays := []multiaddr.Multiaddr{
			mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/1634"),
			mustNewMultiaddr(t, "/ip4/192.168.1.100/tcp/1634"),
			mustNewMultiaddr(t, "/ip6/::1/tcp/1634"),
		}

		addr, err := bzz.NewAddress(signer, underlays, overlay, networkID, nonce)
		if err != nil {
			t.Fatal(err)
		}

		serialized := bzz.SerializeUnderlays(underlays)
		parsed, err := bzz.ParseAddress(serialized, overlay.Bytes(), addr.Signature, nonce, true, networkID)
		if err != nil {
			t.Fatalf("ParseAddress failed: %v", err)
		}

		if !parsed.Overlay.Equal(overlay) {
			t.Errorf("overlay mismatch: got %v, want %v", parsed.Overlay, overlay)
		}

		if len(parsed.Underlays) != 3 {
			t.Errorf("expected 3 underlays, got %d", len(parsed.Underlays))
		}

		if !bzz.AreUnderlaysEqual(parsed.Underlays, underlays) {
			t.Errorf("underlays not equal: got %v, want %v", parsed.Underlays, underlays)
		}
	})

	t.Run("empty underlays - inbound-only peer", func(t *testing.T) {
		t.Parallel()

		// Create address with empty underlays (for inbound-only peers like browsers)
		emptyUnderlays := []multiaddr.Multiaddr{}
		addr, err := bzz.NewAddress(signer, emptyUnderlays, overlay, networkID, nonce)
		if err != nil {
			t.Fatal(err)
		}

		serialized := bzz.SerializeUnderlays(emptyUnderlays)
		parsed, err := bzz.ParseAddress(serialized, overlay.Bytes(), addr.Signature, nonce, true, networkID)
		if err != nil {
			t.Fatalf("ParseAddress failed for empty underlays: %v", err)
		}

		if !parsed.Overlay.Equal(overlay) {
			t.Errorf("overlay mismatch: got %v, want %v", parsed.Overlay, overlay)
		}

		if len(parsed.Underlays) != 0 {
			t.Errorf("expected 0 underlays for inbound-only peer, got %d", len(parsed.Underlays))
		}
	})

	t.Run("without overlay validation", func(t *testing.T) {
		t.Parallel()

		underlay := mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/1634")

		addr, err := bzz.NewAddress(signer, []multiaddr.Multiaddr{underlay}, overlay, networkID, nonce)
		if err != nil {
			t.Fatal(err)
		}

		// Parse without overlay validation
		parsed, err := bzz.ParseAddress(addr.Underlays[0].Bytes(), overlay.Bytes(), addr.Signature, nonce, false, networkID)
		if err != nil {
			t.Fatalf("ParseAddress failed: %v", err)
		}

		if !parsed.Overlay.Equal(overlay) {
			t.Errorf("overlay mismatch: got %v, want %v", parsed.Overlay, overlay)
		}
	})

	t.Run("invalid signature", func(t *testing.T) {
		t.Parallel()

		underlay := mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/1634")
		invalidSignature := make([]byte, 65) // All zeros - invalid signature

		_, err := bzz.ParseAddress(underlay.Bytes(), overlay.Bytes(), invalidSignature, nonce, true, networkID)
		if err == nil {
			t.Error("expected error for invalid signature, got nil")
		}
	})

	t.Run("tampered signature", func(t *testing.T) {
		t.Parallel()

		underlay := mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/1634")

		addr, err := bzz.NewAddress(signer, []multiaddr.Multiaddr{underlay}, overlay, networkID, nonce)
		if err != nil {
			t.Fatal(err)
		}

		// Tamper with signature
		tamperedSig := make([]byte, len(addr.Signature))
		copy(tamperedSig, addr.Signature)
		tamperedSig[0] ^= 0xFF // Flip bits

		_, err = bzz.ParseAddress(addr.Underlays[0].Bytes(), overlay.Bytes(), tamperedSig, nonce, true, networkID)
		if err == nil {
			t.Error("expected error for tampered signature, got nil")
		}
	})

	t.Run("mismatched overlay with validation", func(t *testing.T) {
		t.Parallel()

		underlay := mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/1634")

		addr, err := bzz.NewAddress(signer, []multiaddr.Multiaddr{underlay}, overlay, networkID, nonce)
		if err != nil {
			t.Fatal(err)
		}

		// Use a different overlay
		wrongOverlay := make([]byte, len(overlay.Bytes()))
		copy(wrongOverlay, overlay.Bytes())
		wrongOverlay[0] ^= 0xFF // Flip bits

		_, err = bzz.ParseAddress(addr.Underlays[0].Bytes(), wrongOverlay, addr.Signature, nonce, true, networkID)
		if err == nil {
			t.Error("expected error for mismatched overlay, got nil")
		}
	})

	t.Run("wrong network ID", func(t *testing.T) {
		t.Parallel()

		underlay := mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/1634")

		addr, err := bzz.NewAddress(signer, []multiaddr.Multiaddr{underlay}, overlay, networkID, nonce)
		if err != nil {
			t.Fatal(err)
		}

		// Try to parse with different network ID
		wrongNetworkID := networkID + 1
		_, err = bzz.ParseAddress(addr.Underlays[0].Bytes(), overlay.Bytes(), addr.Signature, nonce, true, wrongNetworkID)
		if err == nil {
			t.Error("expected error for wrong network ID, got nil")
		}
	})

	t.Run("invalid underlay bytes", func(t *testing.T) {
		t.Parallel()

		invalidUnderlay := []byte{0xFF, 0xFF, 0xFF} // Invalid multiaddr bytes

		// We need a valid signature for this test, so create one with valid data first
		underlay := mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/1634")
		addr, err := bzz.NewAddress(signer, []multiaddr.Multiaddr{underlay}, overlay, networkID, nonce)
		if err != nil {
			t.Fatal(err)
		}

		// Now try to parse with invalid underlay bytes
		_, err = bzz.ParseAddress(invalidUnderlay, overlay.Bytes(), addr.Signature, nonce, false, networkID)
		if err == nil {
			t.Error("expected error for invalid underlay bytes, got nil")
		}
	})

	t.Run("ethereum address extraction", func(t *testing.T) {
		t.Parallel()

		underlay := mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/1634")

		addr, err := bzz.NewAddress(signer, []multiaddr.Multiaddr{underlay}, overlay, networkID, nonce)
		if err != nil {
			t.Fatal(err)
		}

		parsed, err := bzz.ParseAddress(addr.Underlays[0].Bytes(), overlay.Bytes(), addr.Signature, nonce, true, networkID)
		if err != nil {
			t.Fatalf("ParseAddress failed: %v", err)
		}

		if len(parsed.EthereumAddress) == 0 {
			t.Error("ethereum address not extracted")
		}

		// Verify ethereum address matches the one from the private key
		expectedEthAddr, err := crypto.NewEthereumAddress(privateKey.PublicKey)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(parsed.EthereumAddress, expectedEthAddr) {
			t.Errorf("ethereum address mismatch: got %x, want %x", parsed.EthereumAddress, expectedEthAddr)
		}
	})
}
