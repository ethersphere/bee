// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package combinator_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/replicas/combinator"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const maxLevel = 8

func TestIterateReplicaAddressesSeq(t *testing.T) {
	t.Run("iterate up to level 0", func(t *testing.T) {
		input := swarm.NewAddress(make([]byte, swarm.HashSize))
		allCombinations := make(map[string]bool)
		count := 0
		maxD := 0
		expectedCount := 0            // No addresses should be returned as level 0 represents no replication.
		expected := map[string]bool{} // Not even the maxLevel-bit-flipped address.

		for combo := range combinator.IterateReplicaAddresses(input, maxD) {
			comboHex := combo.String()
			if allCombinations[comboHex] {
				t.Errorf("Duplicate combination found at count %d: %s", count, comboHex)
			}
			allCombinations[comboHex] = true
			count++
		}

		if count != expectedCount {
			t.Fatalf("Expected to iterate %d times, got %d", expectedCount, count)
		}
		if len(allCombinations) != len(expected) {
			t.Errorf("Mismatched map sizes. Expected %d, got %d", len(expected), len(allCombinations))
		}
		for hexStr := range expected {
			if !allCombinations[hexStr] {
				t.Errorf("Expected combination %s not found in results", hexStr)
			}
		}
	})

	t.Run("iterate up to level 1", func(t *testing.T) {
		input := swarm.NewAddress(make([]byte, swarm.HashSize))
		allCombinations := make(map[string]bool)
		count := 0
		maxD := 1
		expectedCount := 1 << maxD // 2^1 = 2 items
		expected := map[string]bool{
			swarm.NewAddress(append([]byte{0b10000000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=1 (level=1)
			swarm.NewAddress(append([]byte{0b01000000}, make([]byte, swarm.HashSize-1)...)).String(): true, // 2nd bit flipped
		}

		for combo := range combinator.IterateReplicaAddresses(input, maxD) {
			comboHex := combo.String()
			if allCombinations[comboHex] {
				t.Errorf("Duplicate combination found at count %d: %s", count, comboHex)
			}
			allCombinations[comboHex] = true
			count++
		}

		if count != expectedCount {
			t.Fatalf("Expected to iterate %d times, got %d", expectedCount, count)
		}
		if len(allCombinations) != len(expected) {
			t.Errorf("Mismatched map sizes. Expected %d, got %d", len(expected), len(allCombinations))
		}
		for hexStr := range expected {
			if !allCombinations[hexStr] {
				t.Errorf("Expected combination %s not found in results", hexStr)
			}
		}
	})

	t.Run("iterate up to level 2", func(t *testing.T) {
		input := swarm.NewAddress(make([]byte, swarm.HashSize))
		allCombinations := make(map[string]bool)
		count := 0
		maxD := 2
		expectedCount := 1 << maxD // 2^2 = 4 items
		expected := map[string]bool{
			swarm.NewAddress(append([]byte{0b10000000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=1 (level=1)
			swarm.NewAddress(append([]byte{0b01000000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=2 (level=2)
			swarm.NewAddress(append([]byte{0b11000000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=3 (level=2)
			swarm.NewAddress(append([]byte{0b00100000}, make([]byte, swarm.HashSize-1)...)).String(): true, // 3rd bit flipped
		}

		for combo := range combinator.IterateReplicaAddresses(input, maxD) {
			comboHex := combo.String()
			if allCombinations[comboHex] {
				t.Errorf("Duplicate combination found at count %d: %s", count, comboHex)
			}
			allCombinations[comboHex] = true
			count++
		}

		if count != expectedCount {
			t.Fatalf("Expected to iterate %d times, got %d", expectedCount, count)
		}
		if len(allCombinations) != len(expected) {
			t.Errorf("Mismatched map sizes. Expected %d, got %d", len(expected), len(allCombinations))
		}
		for hexStr := range expected {
			if !allCombinations[hexStr] {
				t.Errorf("Expected combination %s not found in results", hexStr)
			}
		}
	})

	t.Run("Iterate up to level=3", func(t *testing.T) {
		input := swarm.NewAddress(make([]byte, swarm.HashSize))
		allCombinations := make(map[string]bool)
		count := 0
		maxD := 3
		expectedCount := 1 << maxD // 2^3 = 8 items
		expected := map[string]bool{
			swarm.NewAddress(append([]byte{0b10000000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=1 (level=1)
			swarm.NewAddress(append([]byte{0b01000000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=2 (level=2)
			swarm.NewAddress(append([]byte{0b11000000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=3 (level=2)
			swarm.NewAddress(append([]byte{0b00100000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=4 (level=3)
			swarm.NewAddress(append([]byte{0b10100000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=5 (level=3)
			swarm.NewAddress(append([]byte{0b01100000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=6 (level=3)
			swarm.NewAddress(append([]byte{0b11100000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=7 (level=3)
			swarm.NewAddress(append([]byte{0b00010000}, make([]byte, swarm.HashSize-1)...)).String(): true, // 4th bit flipped
		}

		for combo := range combinator.IterateReplicaAddresses(input, maxD) {
			comboHex := combo.String()
			if allCombinations[comboHex] {
				t.Errorf("Duplicate combination found at count %d: %s", count, comboHex)
			}
			allCombinations[comboHex] = true
			count++
		}

		if count != expectedCount {
			t.Fatalf("Expected to iterate %d times, got %d", expectedCount, count)
		}

		// Check that the items we got are the ones we expected
		if len(allCombinations) != len(expected) {
			t.Errorf("Mismatched map sizes. Expected %d, got %d", len(expected), len(allCombinations))
		}
		for hexStr := range expected {
			if !allCombinations[hexStr] {
				t.Errorf("Expected combination %s not found in results", hexStr)
			}
		}
	})

	t.Run("iterate up to level 4", func(t *testing.T) {
		input := swarm.NewAddress(make([]byte, swarm.HashSize))
		allCombinations := make(map[string]bool)
		count := 0
		maxD := 4
		expectedCount := 1 << maxD // 2^4 = 16 items
		expected := map[string]bool{
			swarm.NewAddress(append([]byte{0b10000000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=1 (level=1)
			swarm.NewAddress(append([]byte{0b01000000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=2 (level=2)
			swarm.NewAddress(append([]byte{0b11000000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=3 (level=2)
			swarm.NewAddress(append([]byte{0b00100000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=4 (level=3)
			swarm.NewAddress(append([]byte{0b10100000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=5 (level=3)
			swarm.NewAddress(append([]byte{0b01100000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=6 (level=3)
			swarm.NewAddress(append([]byte{0b11100000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=7 (level=3)
			swarm.NewAddress(append([]byte{0b00010000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=8 (level=4)
			swarm.NewAddress(append([]byte{0b10010000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=9 (level=4)
			swarm.NewAddress(append([]byte{0b01010000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=10 (level=4)
			swarm.NewAddress(append([]byte{0b11010000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=11 (level=4)
			swarm.NewAddress(append([]byte{0b00110000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=12 (level=4)
			swarm.NewAddress(append([]byte{0b10110000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=13 (level=4)
			swarm.NewAddress(append([]byte{0b01110000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=14 (level=4)
			swarm.NewAddress(append([]byte{0b11110000}, make([]byte, swarm.HashSize-1)...)).String(): true, // i=15 (level=4)
			swarm.NewAddress(append([]byte{0b00001000}, make([]byte, swarm.HashSize-1)...)).String(): true, // 5th bit flipped
		}

		for combo := range combinator.IterateReplicaAddresses(input, maxD) {
			comboHex := combo.String()
			if allCombinations[comboHex] {
				t.Errorf("Duplicate combination found at count %d: %s", count, comboHex)
			}
			allCombinations[comboHex] = true
			count++
		}

		if count != expectedCount {
			t.Fatalf("Expected to iterate %d times, got %d", expectedCount, count)
		}
		if len(allCombinations) != len(expected) {
			t.Errorf("Mismatched map sizes. Expected %d, got %d", len(expected), len(allCombinations))
		}
		for hexStr := range expected {
			if !allCombinations[hexStr] {
				t.Errorf("Expected combination %s not found in results", hexStr)
			}
		}
	})

	t.Run("maxLevel limits iteration", func(t *testing.T) {
		input := swarm.NewAddress(make([]byte, swarm.HashSize))
		count := 0
		// maxLevel=2 should give 3 items (2^2-1 for levels 1, 2) + 1 for the maxLevel bit flipped address
		expectedCount := 4

		for range combinator.IterateReplicaAddresses(input, 2) {
			count++
		}

		if count != expectedCount {
			t.Errorf("Expected %d items for maxLevel=2, got %d", expectedCount, count)
		}
	})

	t.Run("Iterator stops correctly at end of byte slice", func(t *testing.T) {
		// 1 byte = 8 bits.
		// Iterator should produce 2^8-1 = 255 items (for level=1 through level=8).
		// The 257th item (i=256) would require level=9,
		// which needs 2 bytes. The iterator should stop there.
		input := swarm.NewAddress([]byte{0xDE}) // 1 byte
		expectedCount := (1 << 8) - 1           // 255
		count := 0

		allCombinations := make(map[string]bool)

		for combo := range combinator.IterateReplicaAddresses(input, maxLevel) {
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

	t.Run("level=0 edge case (nil slice)", func(t *testing.T) {
		// The iterator starts at i=1, which needs level=1, which needs 1 byte.
		// A nil slice fails this.
		// So, this should iterate *exactly zero times*.
		var input swarm.Address
		count := 0

		for range combinator.IterateReplicaAddresses(input, maxLevel) {
			count++
		}

		if count != 0 {
			t.Fatalf("Expected exactly 0 items for nil slice, got %d", count)
		}
	})

	t.Run("Consumer stops early (break)", func(t *testing.T) {
		input := swarm.NewAddress(make([]byte, swarm.HashSize))
		count := 0
		stopAt := 5

		seq := combinator.IterateReplicaAddresses(input, maxLevel)
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

	t.Run("iterate with negative level", func(t *testing.T) {
		input := swarm.NewAddress(make([]byte, swarm.HashSize))
		count := 0
		maxD := -1 // Negative level

		for range combinator.IterateReplicaAddresses(input, maxD) {
			count++
		}

		if count != 0 {
			t.Fatalf("Expected to iterate 0 times for negative level, got %d", count)
		}
	})
}

var benchAddress = swarm.NewAddress(append([]byte{0xDE, 0xAD, 0xBE, 0xEF}, make([]byte, swarm.HashSize-4)...))

// runBenchmark is a helper to run the iterator for a fixed level.
func runBenchmark(b *testing.B, maxLevel int) {
	b.Helper()

	// We run the loop b.N times, as required by the benchmark harness.
	for b.Loop() {
		// We use a volatile variable to ensure the loop body
		// (the slice generation) isn't optimized away.
		var volatileAddr swarm.Address

		seq := combinator.IterateReplicaAddresses(benchAddress, maxLevel)
		for combo := range seq {
			volatileAddr = combo
		}

		// To prevent compiler optimizing out the loop if volatileAddr isn't used.
		// This is a common pattern, though often `go:noinline` on a helper
		// function or global assignment is also used.
		if volatileAddr.IsZero() {
			b.Error("volatileAddr should not be nil")
		}
	}
}

// BenchmarkMaxLevel1 iterates over 2^1 = 2 items
func BenchmarkMaxLevel1(b *testing.B) {
	runBenchmark(b, 1)
}

// BenchmarkMaxLevel2 iterates over 2^2 = 4 items
func BenchmarkMaxLevel2(b *testing.B) {
	runBenchmark(b, 2)
}

// BenchmarkMaxLevel3 iterates over 2^3 = 8 items
func BenchmarkMaxLevel3(b *testing.B) {
	runBenchmark(b, 3)
}

// BenchmarkMaxLevel4 iterates over 2^4 = 16 items
func BenchmarkMaxLevel4(b *testing.B) {
	runBenchmark(b, 4)
}

// BenchmarkMaxLevel8 iterates over 2^8 = 256 items
func BenchmarkMaxLevel8(b *testing.B) {
	runBenchmark(b, 8)
}

// BenchmarkMaxLevel12 iterates over 2^12 = 4096 items
func BenchmarkMaxLevel12(b *testing.B) {
	runBenchmark(b, 12)
}

// BenchmarkMaxLevel16 iterates over 2^16 = 65536 items
func BenchmarkMaxLevel16(b *testing.B) {
	runBenchmark(b, 16)
}

// BenchmarkMaxLevel20 iterates over 2^20 = 1,048,576 items
func BenchmarkMaxLevel20(b *testing.B) {
	runBenchmark(b, 20)
}
