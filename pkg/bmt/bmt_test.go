// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/bmt/reference"
	"github.com/ethersphere/bee/pkg/swarm"
	"golang.org/x/sync/errgroup"
)

const (
	// testPoolSize is the number of bmt trees the pool keeps when
	testPoolSize = 16
	// segmentCount is the maximum number of segments of the underlying chunk
	// Should be equal to max-chunk-data-size / hash-size
	// Currently set to 128 == 4096 (default chunk size) / 32 (sha3.keccak256 size)
	testSegmentCount = 128
)

var (
	testSegmentCounts = []int{1, 2, 3, 4, 5, 8, 9, 15, 16, 17, 32, 37, 42, 53, 63, 64, 65, 111, 127, 128}
	hashSize          = swarm.NewHasher().Size()
	testData          = make([]byte, 4096)
	seed              = time.Now().Unix()
)

func refHash(count int, data []byte) ([]byte, error) {
	rbmt := reference.NewRefHasher(swarm.NewHasher(), count)
	refNoMetaHash, err := rbmt.Hash(data)
	if err != nil {
		return nil, err
	}
	return bmt.Sha3hash(bmt.LengthToSpan(int64(len(data))), refNoMetaHash)
}

// syncHash hashes the data and the span using the bmt hasher
func syncHash(h *bmt.Hasher, data []byte) ([]byte, error) {
	h.Reset()
	h.SetHeaderInt64(int64(len(data)))
	_, err := h.Write(data)
	if err != nil {
		return nil, err
	}
	return h.Hash(nil)
}

// tests if hasher responds with correct hash comparing the reference implementation return value
func TestHasherEmptyData(t *testing.T) {
	for _, count := range testSegmentCounts {
		t.Run(fmt.Sprintf("%d_segments", count), func(t *testing.T) {
			expHash, err := refHash(count, nil)
			if err != nil {
				t.Fatal(err)
			}
			pool := bmt.NewPool(bmt.NewConf(swarm.NewHasher, count, 1))
			h := pool.Get()
			resHash, err := syncHash(h, nil)
			if err != nil {
				t.Fatal(err)
			}
			pool.Put(h)
			if !bytes.Equal(expHash, resHash) {
				t.Fatalf("hash mismatch with reference. expected %x, got %x", expHash, resHash)
			}
		})
	}
}

// tests sequential write with entire max size written in one go
func TestSyncHasherCorrectness(t *testing.T) {
	setRandomBytes(t, testData, seed)

	for _, count := range testSegmentCounts {
		t.Run(fmt.Sprintf("segments_%v", count), func(t *testing.T) {
			max := count * hashSize
			var incr int
			capacity := 1
			pool := bmt.NewPool(bmt.NewConf(swarm.NewHasher, count, capacity))
			for n := 0; n <= max; n += incr {
				h := pool.Get()
				incr = 1 + rand.Intn(5)
				err := testHasherCorrectness(h, testData, n, count)
				if err != nil {
					t.Fatalf("seed %d: %v", seed, err)
				}
				pool.Put(h)
			}
		})
	}
}

// tests that the BMT hasher can be synchronously reused with poolsizes 1 and testPoolSize
func TestHasherReuse(t *testing.T) {
	t.Run(fmt.Sprintf("poolsize_%d", 1), func(t *testing.T) {
		testHasherReuse(t, 1)
	})
	t.Run(fmt.Sprintf("poolsize_%d", testPoolSize), func(t *testing.T) {
		testHasherReuse(t, testPoolSize)
	})
}

// tests if bmt reuse is not corrupting result
func testHasherReuse(t *testing.T, poolsize int) {
	pool := bmt.NewPool(bmt.NewConf(swarm.NewHasher, testSegmentCount, poolsize))
	h := pool.Get()
	defer pool.Put(h)

	for i := 0; i < 100; i++ {
		seed := int64(i)
		setRandomBytes(t, testData, seed)
		n := rand.Intn(h.Capacity())
		err := testHasherCorrectness(h, testData, n, testSegmentCount)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
	}
}

// tests if pool can be cleanly reused even in concurrent use by several hashers
func TestBMTConcurrentUse(t *testing.T) {
	setRandomBytes(t, testData, seed)
	pool := bmt.NewPool(bmt.NewConf(swarm.NewHasher, testSegmentCount, testPoolSize))
	cycles := 100

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	eg, ectx := errgroup.WithContext(ctx)
	for i := 0; i < cycles; i++ {
		eg.Go(func() error {
			select {
			case <-ectx.Done():
				return ectx.Err()
			default:
			}
			h := pool.Get()
			defer pool.Put(h)

			n := rand.Intn(h.Capacity())
			return testHasherCorrectness(h, testData, n, testSegmentCount)
		})
	}
	if err := eg.Wait(); err != nil {
		t.Fatalf("seed %d: %v", seed, err)
	}
}

// tests BMT Hasher io.Writer interface is working correctly even with random short writes
func TestBMTWriterBuffers(t *testing.T) {
	for i, count := range testSegmentCounts {
		t.Run(fmt.Sprintf("%d_segments", count), func(t *testing.T) {
			pool := bmt.NewPool(bmt.NewConf(swarm.NewHasher, count, testPoolSize))
			h := pool.Get()
			defer pool.Put(h)

			size := h.Capacity()
			seed := int64(i)
			setRandomBytes(t, testData, seed)

			resHash, err := syncHash(h, testData[:size])
			if err != nil {
				t.Fatal(err)
			}
			expHash, err := refHash(count, testData[:size])
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(resHash, expHash) {
				t.Fatalf("single write :hash mismatch with reference. expected %x, got %x", expHash, resHash)
			}
			attempts := 10
			f := func() error {
				h := pool.Get()
				defer pool.Put(h)

				reads := rand.Intn(count*2-1) + 1
				offsets := make([]int, reads+1)
				for i := 0; i < reads; i++ {
					offsets[i] = rand.Intn(size) + 1
				}
				offsets[reads] = size
				from := 0
				sort.Ints(offsets)
				for _, to := range offsets {
					if from < to {
						read, err := h.Write(testData[from:to])
						if err != nil {
							return err
						}
						if read != to-from {
							return fmt.Errorf("incorrect read. expected %v bytes, got %v", to-from, read)
						}
						from = to
					}
				}
				h.SetHeaderInt64(int64(size))
				resHash, err := h.Hash(nil)
				if err != nil {
					return err
				}
				if !bytes.Equal(resHash, expHash) {
					return fmt.Errorf("hash mismatch on %v. expected %x, got %x", offsets, expHash, resHash)
				}
				return nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			eg, ectx := errgroup.WithContext(ctx)
			for i := 0; i < attempts; i++ {
				eg.Go(func() error {
					select {
					case <-ectx.Done():
						return ectx.Err()
					default:
					}
					return f()
				})
			}
			if err := eg.Wait(); err != nil {
				t.Fatalf("seed %d: %v", seed, err)
			}
		})
	}
}

// helper function that compares reference and optimised implementations for correctness
func testHasherCorrectness(h *bmt.Hasher, data []byte, n, count int) (err error) {
	if len(data) < n {
		n = len(data)
	}
	exp, err := refHash(count, data[:n])
	if err != nil {
		return err
	}
	got, err := syncHash(h, data[:n])
	if err != nil {
		return err
	}
	if !bytes.Equal(got, exp) {
		return fmt.Errorf("wrong hash: expected %x, got %x", exp, got)
	}
	return nil
}

// verifies that the bmt.Hasher can be used with the hash.Hash interface
func TestUseSyncAsOrdinaryHasher(t *testing.T) {
	pool := bmt.NewPool(bmt.NewConf(swarm.NewHasher, testSegmentCount, testPoolSize))
	h := pool.Get()
	defer pool.Put(h)
	data := []byte("moodbytesmoodbytesmoodbytesmoodbytes")
	expHash, err := refHash(128, data)
	if err != nil {
		t.Fatal(err)
	}
	h.SetHeaderInt64(int64(len(data)))
	_, err = h.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	resHash := h.Sum(nil)
	if !bytes.Equal(expHash, resHash) {
		t.Fatalf("normalhash; expected %x, got %x", expHash, resHash)
	}
}

func setRandomBytes(t testing.TB, data []byte, seed int64) {
	t.Helper()
	s := rand.NewSource(seed)
	r := rand.New(s)
	_, err := io.ReadFull(r, data)
	if err != nil {
		t.Fatal(err)
	}
}
