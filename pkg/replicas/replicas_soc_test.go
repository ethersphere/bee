// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package replicas

import (
	"fmt"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestMirrorBitsToMSB(t *testing.T) {
	t.Parallel()

	tests := []struct {
		v        byte
		n        uint8
		expected byte
	}{
		{0b00001101, 4, 0b10110000}, // Example from comment
		{0b00000001, 1, 0b10000000},
		{0b00001111, 4, 0b11110000},
		{0b00000000, 4, 0b00000000},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("v=%b_n=%d", tt.v, tt.n), func(t *testing.T) {
			t.Parallel()
			if got := mirrorBitsToMSB(tt.v, tt.n); got != tt.expected {
				t.Errorf("mirrorBitsToMSB(%b, %d) = %b, want %b", tt.v, tt.n, got, tt.expected)
			}
		})
	}
}

func TestCountBitsRequired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		v        uint8
		expected uint8
	}{
		{0, 1},   // Special case
		{1, 1},   // 1 bit
		{3, 2},   // 2 bits
		{7, 3},   // 3 bits
		{15, 4},  // 4 bits
		{255, 8}, // 8 bits
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("v=%d", tt.v), func(t *testing.T) {
			t.Parallel()
			if got := countBitsRequired(tt.v); got != tt.expected {
				t.Errorf("countBitsRequired(%d) = %d, want %d", tt.v, got, tt.expected)
			}
		})
	}
}

func TestReplicate_Line48(t *testing.T) {
	t.Parallel()

	// Test line 48: addr[0] &= 0xFF >> bitsRequired
	// This clears the first bitsRequired MSBs
	baseAddr := swarm.MustParseHexAddress("FF00000000000000000000000000000000000000000000000000000000000000")

	tests := []struct {
		bitsRequired uint8
		expectedMask byte
	}{
		{1, 0x7F}, // 0b01111111
		{2, 0x3F}, // 0b00111111
		{4, 0x0F}, // 0b00001111
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("bits=%d", tt.bitsRequired), func(t *testing.T) {
			t.Parallel()
			// Test the mask calculation
			mask := byte(0xFF >> tt.bitsRequired)
			if mask != tt.expectedMask {
				t.Errorf("mask = %b, want %b", mask, tt.expectedMask)
			}

			// Test that applying the mask clears the MSBs
			addr := make([]byte, 32)
			copy(addr, baseAddr.Bytes())
			addr[0] &= mask

			if addr[0] != tt.expectedMask {
				t.Errorf("after mask: addr[0] = %b, want %b", addr[0], tt.expectedMask)
			}

			// Verify MSBs are cleared
			msbMask := byte(0xFF) << (8 - tt.bitsRequired)
			if addr[0]&msbMask != 0 {
				t.Errorf("first %d bits should be zero, got %b", tt.bitsRequired, addr[0])
			}
		})
	}
}

func TestReplicate(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.MustParseHexAddress("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")
	replicator := &socReplicator{addr: baseAddr.Bytes(), rLevel: redundancy.MEDIUM}

	tests := []struct {
		i            uint8
		bitsRequired uint8
	}{
		{0, 1},
		{1, 2},
		{3, 4},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("i=%d_bits=%d", tt.i, tt.bitsRequired), func(t *testing.T) {
			t.Parallel()
			replica := replicator.replicate(tt.i, tt.bitsRequired)

			// Verify nonce matches first byte
			if replica.nonce != replica.addr[0] {
				t.Errorf("nonce = %d, want %d", replica.nonce, replica.addr[0])
			}

			// Verify remaining bytes unchanged
			for i := 1; i < len(replica.addr); i++ {
				if replica.addr[i] != baseAddr.Bytes()[i] {
					t.Errorf("byte[%d] changed: got %d, want %d", i, replica.addr[i], baseAddr.Bytes()[i])
				}
			}

			// Verify first byte differs from original (or was modified)
			if replica.addr[0] == baseAddr.Bytes()[0] {
				// This is okay if the code explicitly handles this case (line 50-52)
				// But we should verify the logic worked
				mask := byte(0xFF >> tt.bitsRequired)
				mirroredBits := mirrorBitsToMSB(tt.i, tt.bitsRequired)
				expected := (baseAddr.Bytes()[0] & mask) | mirroredBits
				if expected == baseAddr.Bytes()[0] {
					// Original would have been flipped, so replica should differ
					if replica.addr[0] == baseAddr.Bytes()[0] {
						t.Errorf("replica first byte should differ from original")
					}
				}
			}
		})
	}
}

func TestReplicas(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.MustParseHexAddress("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")

	for _, rLevel := range []redundancy.Level{redundancy.MEDIUM, redundancy.STRONG, redundancy.INSANE, redundancy.DefaultLevel} {
		t.Run(fmt.Sprintf("level_%d", rLevel), func(t *testing.T) {
			t.Parallel()

			replicator := newSocReplicator(baseAddr, rLevel)
			var replicas []*socReplica
			for r := range replicator.c {
				replicas = append(replicas, r)
			}

			// Verify count
			if len(replicas) != rLevel.GetReplicaCount() {
				t.Errorf("got %d replicas, want %d", len(replicas), rLevel.GetReplicaCount())
			}

			// Verify structure and uniqueness
			seen := make(map[string]bool)
			for i, r := range replicas {
				if len(r.addr) != 32 {
					t.Errorf("replica %d: invalid address length", i)
				}
				if r.nonce != r.addr[0] {
					t.Errorf("replica %d: nonce mismatch", i)
				}
				// Verify remaining bytes unchanged
				for j := 1; j < 32; j++ {
					if r.addr[j] != baseAddr.Bytes()[j] {
						t.Errorf("replica %d: byte[%d] changed", i, j)
					}
				}
				// Check uniqueness
				addrStr := string(r.addr)
				if seen[addrStr] {
					t.Errorf("replica %d: duplicate address", i)
				}
				seen[addrStr] = true
			}

			// Verify dispersion (at least some first bytes differ)
			firstBytes := make(map[byte]bool)
			for _, r := range replicas {
				firstBytes[r.addr[0]] = true
			}
			if len(firstBytes) < len(replicas)/2 {
				t.Errorf("poor dispersion: only %d unique first bytes", len(firstBytes))
			}
		})
	}
}
