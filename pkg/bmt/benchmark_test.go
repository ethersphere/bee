// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt_test

import (
	crand "crypto/rand"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/bmt/reference"
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
	caps := []int{1, testPoolSize}
	for size := 4096; size >= 128; size /= 2 {
		for _, c := range caps {
			t.Run(fmt.Sprintf("poolsize_%v_size_%v", c, size), func(t *testing.B) {
				benchmarkPool(t, c, size)
			})
		}
	}
}

// benchmarks simple sha3 hash on chunks
func benchmarkSHA3(t *testing.B, n int) {
	data := make([]byte, n)
	_, err := io.ReadFull(crand.Reader, data)
	if err != nil {
		t.Fatal(err)
	}

	t.ReportAllocs()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		_ = sha3hash(data)
	}
}

// benchmarks the minimum hashing time for a balanced (for simplicity) BMT
// by doing count/segmentsize parallel hashings of 2*segmentsize bytes
// doing it on n testPoolSize each reusing the base hasher
// the premise is that this is the minimum computation needed for a BMT
// therefore this serves as a theoretical optimum for concurrent implementations
func benchmarkBMTBaseline(t *testing.B, n int) {
	hashSize := testHasher().Size()
	data := make([]byte, hashSize)
	_, err := io.ReadFull(crand.Reader, data)
	if err != nil {
		t.Fatal(err)
	}

	t.ReportAllocs()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		count := int32((n-1)/hashSize + 1)
		wg := sync.WaitGroup{}
		wg.Add(testPoolSize)
		var i int32
		for j := 0; j < testPoolSize; j++ {
			go func() {
				defer wg.Done()
				for atomic.AddInt32(&i, 1) < count {
					_ = sha3hash(data)
				}
			}()
		}
		wg.Wait()
	}
}

// benchmarks BMT Hasher
func benchmarkBMT(t *testing.B, n int) {
	data := make([]byte, n)
	_, err := io.ReadFull(crand.Reader, data)
	if err != nil {
		t.Fatal(err)
	}
	pool := bmt.NewPool(bmt.NewConf(testHasher, testSegmentCount, testPoolSize))
	h := pool.Get()
	defer pool.Put(h)

	t.ReportAllocs()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		_, err = syncHash(h, 0, data)
		if err != nil {
			t.Fatal(err)
		}
	}
}

// benchmarks 100 concurrent bmt hashes with pool capacity
func benchmarkPool(t *testing.B, poolsize, n int) {
	data := make([]byte, n)
	_, err := io.ReadFull(crand.Reader, data)
	if err != nil {
		t.Fatal(err)
	}
	pool := bmt.NewPool(bmt.NewConf(testHasher, testSegmentCount, poolsize))
	cycles := 100

	t.ReportAllocs()
	t.ResetTimer()
	wg := sync.WaitGroup{}
	for i := 0; i < t.N; i++ {
		wg.Add(cycles)
		for j := 0; j < cycles; j++ {
			go func() {
				defer wg.Done()
				h := pool.Get()
				defer pool.Put(h)
				_, _ = syncHash(h, 0, data)
			}()
		}
		wg.Wait()
	}
}

// benchmarks the reference hasher
func benchmarkRefHasher(t *testing.B, n int) {
	data := make([]byte, n)
	_, err := io.ReadFull(crand.Reader, data)
	if err != nil {
		t.Fatal(err)
	}
	rbmt := reference.NewRefHasher(testHasher(), 128)

	t.ReportAllocs()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		_, err := rbmt.Hash(data)
		if err != nil {
			t.Fatal(err)
		}
	}
}
