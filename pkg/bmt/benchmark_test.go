// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt_test

import (
	"fmt"
	"testing"

	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/bmt/reference"
	"github.com/ethersphere/bee/pkg/swarm"
	"golang.org/x/sync/errgroup"
)

//
func BenchmarkBMT(t *testing.B) {
	for size := 4096; size >= 128; size /= 2 {
		t.Run(fmt.Sprintf("%v_size_%v", "SHA3", size), func(t *testing.B) {
			benchmarkSHA3(t, size)
		})
		t.Run(fmt.Sprintf("%v_size_%v", "Baseline", size), func(t *testing.B) {
			benchmarkBMTBaseline(t, size)
		})
		t.Run(fmt.Sprintf("%v_size_%v", "REF", size), func(t *testing.B) {
			benchmarkRefHasher(t, size)
		})
		t.Run(fmt.Sprintf("%v_size_%v", "BMT", size), func(t *testing.B) {
			benchmarkBMT(t, size)
		})
	}
}

func BenchmarkPool(t *testing.B) {
	for _, c := range []int{1, 8, 16, 32, 64} {
		t.Run(fmt.Sprintf("poolsize_%v", c), func(t *testing.B) {
			benchmarkPool(t, c)
		})
	}
}

// benchmarks simple sha3 hash on chunks
func benchmarkSHA3(t *testing.B, n int) {
	setRandomBytes(t, testData, seed)

	t.ReportAllocs()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		if _, err := bmt.Sha3hash(testData[:n]); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
	}
}

// benchmarks the minimum hashing time for a balanced (for simplicity) BMT
// by doing count/segmentsize parallel hashings of 2*segmentsize bytes
// doing it on n testPoolSize each reusing the base hasher
// the premise is that this is the minimum computation needed for a BMT
// therefore this serves as a theoretical optimum for concurrent implementations
func benchmarkBMTBaseline(t *testing.B, n int) {
	setRandomBytes(t, testData, seed)

	t.ReportAllocs()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		eg := new(errgroup.Group)
		for j := 0; j < testSegmentCount; j++ {
			eg.Go(func() error {
				_, err := bmt.Sha3hash(testData[:hashSize])
				return err
			})
		}
		if err := eg.Wait(); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
	}
}

// benchmarks BMT Hasher
func benchmarkBMT(t *testing.B, n int) {
	setRandomBytes(t, testData, seed)

	pool := bmt.NewPool(bmt.NewConf(swarm.NewHasher, testSegmentCount, testPoolSize))
	h := pool.Get()
	defer pool.Put(h)

	t.ReportAllocs()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		if _, err := syncHash(h, n, testData); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
	}
}

// benchmarks 100 concurrent bmt hashes with pool capacity
func benchmarkPool(t *testing.B, poolsize int) {
	setRandomBytes(t, testData, seed)

	pool := bmt.NewPool(bmt.NewConf(swarm.NewHasher, testSegmentCount, poolsize))
	cycles := 100

	t.ReportAllocs()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		eg := new(errgroup.Group)
		for j := 0; j < cycles; j++ {
			eg.Go(func() error {
				h := pool.Get()
				defer pool.Put(h)
				_, err := syncHash(h, h.Capacity(), testData)
				return err
			})
		}
		if err := eg.Wait(); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
	}
}

// benchmarks the reference hasher
func benchmarkRefHasher(t *testing.B, n int) {
	setRandomBytes(t, testData, seed)

	rbmt := reference.NewRefHasher(swarm.NewHasher(), 128)

	t.ReportAllocs()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		_, err := rbmt.Hash(testData[:n])
		if err != nil {
			t.Fatal(err)
		}
	}
}
