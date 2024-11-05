// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt_test

import (
	"fmt"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bmt"
	"github.com/ethersphere/bee/v2/pkg/bmt/reference"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	"golang.org/x/sync/errgroup"
)

func BenchmarkBMT(b *testing.B) {
	for size := 4096; size >= 128; size /= 2 {
		b.Run(fmt.Sprintf("%v_size_%v", "SHA3", size), func(b *testing.B) {
			benchmarkSHA3(b, size)
		})
		b.Run(fmt.Sprintf("%v_size_%v", "Baseline", size), func(b *testing.B) {
			benchmarkBMTBaseline(b, size)
		})
		b.Run(fmt.Sprintf("%v_size_%v", "REF", size), func(b *testing.B) {
			benchmarkRefHasher(b, size)
		})
		b.Run(fmt.Sprintf("%v_size_%v", "BMT", size), func(b *testing.B) {
			benchmarkBMT(b, size)
		})
	}
}

func BenchmarkPool(b *testing.B) {
	for _, c := range []int{1, 8, 16, 32, 64} {
		b.Run(fmt.Sprintf("poolsize_%v", c), func(b *testing.B) {
			benchmarkPool(b, c)
		})
	}
}

// benchmarks simple sha3 hash on chunks
func benchmarkSHA3(b *testing.B, n int) {
	b.Helper()

	testData := testutil.RandBytesWithSeed(b, 4096, seed)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := bmt.Sha3hash(testData[:n]); err != nil {
			b.Fatalf("seed %d: %v", seed, err)
		}
	}
}

// benchmarks the minimum hashing time for a balanced (for simplicity) BMT
// by doing count/segmentsize parallel hashings of 2*segmentsize bytes
// doing it on n testPoolSize each reusing the base hasher
// the premise is that this is the minimum computation needed for a BMT
// therefore this serves as a theoretical optimum for concurrent implementations
func benchmarkBMTBaseline(b *testing.B, _ int) {
	b.Helper()

	testData := testutil.RandBytesWithSeed(b, 4096, seed)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eg := new(errgroup.Group)
		for j := 0; j < testSegmentCount; j++ {
			eg.Go(func() error {
				_, err := bmt.Sha3hash(testData[:hashSize])
				return err
			})
		}
		if err := eg.Wait(); err != nil {
			b.Fatalf("seed %d: %v", seed, err)
		}
	}
}

// benchmarks BMT Hasher
func benchmarkBMT(b *testing.B, n int) {
	b.Helper()

	testData := testutil.RandBytesWithSeed(b, 4096, seed)

	pool := bmt.NewPool(bmt.NewConf(swarm.NewHasher, testSegmentCount, testPoolSize))
	h := pool.Get()
	defer pool.Put(h)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := syncHash(h, testData[:n]); err != nil {
			b.Fatalf("seed %d: %v", seed, err)
		}
	}
}

// benchmarks 100 concurrent bmt hashes with pool capacity
func benchmarkPool(b *testing.B, poolsize int) {
	b.Helper()

	testData := testutil.RandBytesWithSeed(b, 4096, seed)

	pool := bmt.NewPool(bmt.NewConf(swarm.NewHasher, testSegmentCount, poolsize))
	cycles := 100

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eg := new(errgroup.Group)
		for j := 0; j < cycles; j++ {
			eg.Go(func() error {
				h := pool.Get()
				defer pool.Put(h)
				_, err := syncHash(h, testData[:h.Capacity()])
				return err
			})
		}
		if err := eg.Wait(); err != nil {
			b.Fatalf("seed %d: %v", seed, err)
		}
	}
}

// benchmarks the reference hasher
func benchmarkRefHasher(b *testing.B, n int) {
	b.Helper()

	testData := testutil.RandBytesWithSeed(b, 4096, seed)

	rbmt := reference.NewRefHasher(swarm.NewHasher(), 128)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := rbmt.Hash(testData[:n])
		if err != nil {
			b.Fatal(err)
		}
	}
}
