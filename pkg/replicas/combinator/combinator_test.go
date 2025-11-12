// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package combinator_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/replicas/combinator"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const maxDepth = 8

func TestIterateAddressCombinationsSeq(t *testing.T) {
	t.Run("Iterate up to depth=3", func(t *testing.T) {
		input := swarm.NewAddress(make([]byte, swarm.HashSize))
		allCombinations := make(map[string]bool)
		count := 0
		maxItems := 9 // 2^3 (which covers depth=0, 1, 2, 3) + 1 for the maxDepth+1 bit flipped address

		// These are the 8 combinations we expect for depth=3
		expected := addressesToHexMap([]swarm.Address{
			swarm.NewAddress(append([]byte{0b00000000}, make([]byte, swarm.HashSize-1)...)), // i=0 (depth=0)
			swarm.NewAddress(append([]byte{0b10000000}, make([]byte, swarm.HashSize-1)...)), // i=1 (depth=1)
			swarm.NewAddress(append([]byte{0b01000000}, make([]byte, swarm.HashSize-1)...)), // i=2 (depth=2)
			swarm.NewAddress(append([]byte{0b11000000}, make([]byte, swarm.HashSize-1)...)), // i=3 (depth=2)
			swarm.NewAddress(append([]byte{0b00100000}, make([]byte, swarm.HashSize-1)...)), // i=4 (depth=3)
			swarm.NewAddress(append([]byte{0b10100000}, make([]byte, swarm.HashSize-1)...)), // i=5 (depth=3)
			swarm.NewAddress(append([]byte{0b01100000}, make([]byte, swarm.HashSize-1)...)), // i=6 (depth=3)
			swarm.NewAddress(append([]byte{0b11100000}, make([]byte, swarm.HashSize-1)...)), // i=7 (depth=3)
			swarm.NewAddress(append([]byte{0b00010000}, make([]byte, swarm.HashSize-1)...)), // i=8 (depth=3)
		})

		for combo := range combinator.IterateAddressCombinations(input, 3) {
			comboHex := combo.String()
			if allCombinations[comboHex] {
				t.Errorf("Duplicate combination found at count %d: %s", count, comboHex)
			}
			allCombinations[comboHex] = true
			count++
		}

		if count != maxItems {
			t.Fatalf("Expected to iterate %d times, got %d", maxItems, count)
		}

		// Check that the 8 items we got are the 8 we expected
		if len(allCombinations) != len(expected) {
			t.Errorf("Mismatched map sizes. Expected %d, got %d", len(expected), len(allCombinations))
		}
		for hexStr := range expected {
			if !allCombinations[hexStr] {
				t.Errorf("Expected combination %s not found in results", hexStr)
			}
		}
	})

	t.Run("maxDepth limits iteration", func(t *testing.T) {
		input := swarm.NewAddress(make([]byte, swarm.HashSize))
		count := 0
		// maxDepth=2 should give 4 items (2^2 for depths 0, 1, 2) + 1 for the maxDepth bit flipped address
		expectedCount := 5

		for range combinator.IterateAddressCombinations(input, 2) {
			count++
		}

		if count != expectedCount {
			t.Errorf("Expected %d items for maxDepth=2, got %d", expectedCount, count)
		}
	})

	t.Run("Iterator stops correctly at end of byte slice", func(t *testing.T) {
		// 1 byte = 8 bits.
		// Iterator should produce 2^8 = 256 items (for depth=0 through depth=8).
		// The 257th item (i=256) would require depth=9,
		// which needs 2 bytes. The iterator should stop there.
		input := swarm.NewAddress([]byte{0xDE}) // 1 byte
		expectedCount := 1 << 8                 // 256
		count := 0

		allCombinations := make(map[string]bool)

		for combo := range combinator.IterateAddressCombinations(input, maxDepth) {
			// Just in case, prevent infinite loop in test
			if count > expectedCount {
				t.Fatalf("Iterator produced more than %d items, count=%d", expectedCount, count)
				break
			}
			comboHex := combo.String()
			if allCombinations[comboHex] {
				t.Errorf("Duplicate combination found: %s", comboHex)
			}
			allCombinations[comboHex] = true
			count++
		}

		if count != expectedCount {
			t.Errorf("Expected exactly %d items for 1 byte, got %d", expectedCount, count)
		}
	})

	t.Run("depth=0 edge case (nil slice)", func(t *testing.T) {
		// depth=0 is i=0. This needs 0 bytes, which a nil slice has.
		// The *next* item, i=1, needs depth=1, which needs 1 byte.
		// A nil slice fails this.
		// So, this should iterate *exactly once*.
		var input swarm.Address
		count := 0
		var firstCombo swarm.Address

		for combo := range combinator.IterateAddressCombinations(input, maxDepth) {
			if count == 0 {
				firstCombo = combo
			}
			count++
		}

		if count != 1 {
			t.Fatalf("Expected exactly 1 item (depth=0) for nil slice, got %d", count)
		}
		// A copy of a nil slice is a non-nil, zero-length slice
		if len(firstCombo.Bytes()) != 0 {
			t.Errorf("Expected first item to be empty slice, got %x", firstCombo.Bytes())
		}
	})

	t.Run("Consumer stops early (break)", func(t *testing.T) {
		input := swarm.NewAddress(make([]byte, swarm.HashSize))
		count := 0
		stopAt := 5

		seq := combinator.IterateAddressCombinations(input, maxDepth)
		for range seq {
			count++
			if count == stopAt {
				break
			}
		}

		if count != stopAt {
			t.Errorf("Expected loop to run %d times, got %d", stopAt, count)
		}
		// This test just proves the 'break' is correctly handled
		// by the iterator's `if !yield(newSlice)` check.
	})
}

var benchAddress = swarm.NewAddress(append([]byte{0xDE, 0xAD, 0xBE, 0xEF}, make([]byte, swarm.HashSize-4)...))

// runBenchmark is a helper to run the iterator for a fixed number of items.
func runBenchmark(b *testing.B, items int) {
	b.Helper()

	// We run the loop b.N times, as required by the benchmark harness.
	for b.Loop() {
		count := 0
		// We use a volatile variable to ensure the loop body
		// (the slice generation) isn't optimized away.
		var volatileAddr swarm.Address

		seq := combinator.IterateAddressCombinations(benchAddress, maxDepth)
		for combo := range seq {
			volatileAddr = combo
			count++
			if count == items {
				break
			}
		}

		// To prevent compiler optimizing out the loop if volatileAddr isn't used.
		// This is a common pattern, though often `go:noinline` on a helper
		// function or global assignment is also used.
		if volatileAddr.IsZero() {
			b.Error("volatileAddr should not be nil")
		}
	}
}

// BenchmarkDepth1 iterates over 2^1 = 2 items
func BenchmarkDepth1(b *testing.B) {
	runBenchmark(b, 1<<1)
}

// BenchmarkDepth2 iterates over 2^2 = 4 items
func BenchmarkDepth2(b *testing.B) {
	runBenchmark(b, 1<<2)
}

// BenchmarkDepth3 iterates over 2^3 = 8 items
func BenchmarkDepth3(b *testing.B) {
	runBenchmark(b, 1<<3)
}

// BenchmarkDepth4 iterates over 2^4 = 16 items
func BenchmarkDepth4(b *testing.B) {
	runBenchmark(b, 1<<4)
}

// BenchmarkDepth8 iterates over 2^8 = 256 items
func BenchmarkDepth8(b *testing.B) {
	runBenchmark(b, 1<<8)
}

// BenchmarkDepth12 iterates over 2^12 = 4096 items
func BenchmarkDepth12(b *testing.B) {
	runBenchmark(b, 1<<12)
}

// BenchmarkDepth16 iterates over 2^16 = 65536 items
func BenchmarkDepth16(b *testing.B) {
	runBenchmark(b, 1<<16)
}

// BenchmarkDepth20 iterates over 2^20 = 1,048,576 items
func BenchmarkDepth20(b *testing.B) {
	runBenchmark(b, 1<<20)
}

// addressesToHexMap is a helper to convert a slice of addresses to a map of hex strings.
func addressesToHexMap(addresses []swarm.Address) map[string]bool {
	set := make(map[string]bool, len(addresses))
	for _, s := range addresses {
		set[s.String()] = true
	}
	return set
}
