// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt_test

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"fmt"
	"io"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/bmt/reference"
	"golang.org/x/crypto/sha3"
	"golang.org/x/sync/errgroup"
)

const (
	// testPoolSize is the number of bmt trees the pool keeps when
	testPoolSize = 16

	// the actual data length generated (could be longer than max datalength of the BMT)
	BufferSize = 4128

	// segmentCount is the maximum number of segments of the underlying chunk
	// Should be equal to max-chunk-data-size / hash-size
	// Currently set to 128 == 4096 (default chunk size) / 32 (sha3.keccak256 size)
	testSegmentCount = 128
)

var (
	testSegmentCounts = []int{1, 2, 3, 4, 5, 8, 9, 15, 16, 17, 32, 37, 42, 53, 63, 64, 65, 111, 127, 128}
	testHasher        = sha3.NewLegacyKeccak256
)

// calculates the Keccak256 SHA3 hash of the data
func sha3hash(data ...[]byte) []byte {
	h := sha3.NewLegacyKeccak256()
	return bmt.DoSum(h, nil, data...)
}

func refHash(count, n int, data []byte) ([]byte, error) {
	rbmt := reference.NewRefHasher(testHasher(), count)
	refNoMetaHash, err := rbmt.Hash(data)
	if err != nil {
		return nil, err
	}
	span := make([]byte, bmt.SpanSize)
	bmt.LengthToSpan(span, int64(n))
	return sha3hash(span, refNoMetaHash), nil
}

// Hash hashes the data and the span using the bmt hasher
func syncHash(h *bmt.Hasher, n int, data []byte) ([]byte, error) {
	h.Reset()
	h.SetSpan(int64(n))
	_, err := h.Write(data)
	if err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

// tests if hasher responds with correct hash comparing the reference implementation return value
func TestHasherEmptyData(t *testing.T) {
	var data []byte
	for _, count := range testSegmentCounts {
		t.Run(fmt.Sprintf("%d_segments", count), func(t *testing.T) {
			pool := bmt.NewPool(bmt.NewConf(testHasher, count, 1))
			h := pool.Get()
			defer pool.Put(h)
			rbmt := reference.NewRefHasher(testHasher(), count)
			expHash, err := rbmt.Hash(data)
			if err != nil {
				t.Fatal(err)
			}
			resHash, err := syncHash(h, 0, data)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(expHash, resHash) {
				t.Fatalf("hash mismatch with reference. expected %x, got %x", resHash, expHash)
			}
		})
	}
}

// tests sequential write with entire max size written in one go
func TestSyncHasherCorrectness(t *testing.T) {
	data := make([]byte, BufferSize)
	_, err := io.ReadFull(crand.Reader, data)
	if err != nil {
		t.Fatal(err)
	}
	size := testHasher().Size()

	for _, count := range testSegmentCounts {
		t.Run(fmt.Sprintf("segments_%v", count), func(t *testing.T) {
			max := count * size
			var incr int
			capacity := 1
			pool := bmt.NewPool(bmt.NewConf(testHasher, count, capacity))
			h := pool.Get()
			defer pool.Put(h)
			for n := 0; n <= max; n += incr {
				incr = 1 + rand.Intn(5)
				err = testHasherCorrectness(h, testHasher, data, n, count)
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// Tests that the BMT hasher can be synchronously reused with poolsizes 1 and testPoolSize
func TestHasherReuse(t *testing.T) {
	t.Run(fmt.Sprintf("poolsize_%d", 1), func(t *testing.T) {
		testHasherReuse(1, t)
	})
	t.Run(fmt.Sprintf("poolsize_%d", testPoolSize), func(t *testing.T) {
		testHasherReuse(testPoolSize, t)
	})
}

// tests if bmt reuse is not corrupting result
func testHasherReuse(poolsize int, t *testing.T) {
	pool := bmt.NewPool(bmt.NewConf(testHasher, testSegmentCount, poolsize))
	h := pool.Get()
	defer pool.Put(h)

	for i := 0; i < 100; i++ {
		data := make([]byte, BufferSize)
		_, err := io.ReadFull(crand.Reader, data)
		if err != nil {
			t.Fatal(err)
		}
		n := rand.Intn(h.Size())
		err = testHasherCorrectness(h, testHasher, data, n, testSegmentCount)
		if err != nil {
			t.Fatal(err)
		}
	}
}

// Tests if pool can be cleanly reused even in concurrent use by several hasher
func TestBMTConcurrentUse(t *testing.T) {
	pool := bmt.NewPool(bmt.NewConf(testHasher, testSegmentCount, testPoolSize))
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
			data := make([]byte, BufferSize)
			_, err := io.ReadFull(crand.Reader, data)
			if err != nil {
				return err
			}
			n := rand.Intn(h.Size())
			return testHasherCorrectness(h, testHasher, data, n, 128)
		})
	}
	if err := eg.Wait(); err != nil {
		t.Fatal(err)
	}
}

// Tests BMT Hasher io.Writer interface is working correctly
// even multiple short random write buffers
func TestBMTWriterBuffers(t *testing.T) {
	for _, count := range testSegmentCounts {
		t.Run(fmt.Sprintf("%d_segments", count), func(t *testing.T) {
			pool := bmt.NewPool(bmt.NewConf(testHasher, count, testPoolSize))
			h := pool.Get()
			defer pool.Put(h)

			span := h.Capacity()
			data := make([]byte, span)
			_, err := io.ReadFull(crand.Reader, data)
			if err != nil {
				t.Fatal(err)
			}
			resHash, err := syncHash(h, span, data)
			if err != nil {
				t.Fatal(err)
			}
			expHash, err := refHash(count, span, data)
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
				h.Reset()
				reads := rand.Intn(count*2-1) + 1
				offsets := make([]int, reads+1)
				for i := 0; i < reads; i++ {
					offsets[i] = rand.Intn(span) + 1
				}
				offsets[reads] = span
				from := 0
				sort.Ints(offsets)
				for _, to := range offsets {
					if from < to {
						read, err := h.Write(data[from:to])
						if err != nil {
							return err
						}
						if read != to-from {
							return fmt.Errorf("incorrect read. expected %v bytes, got %v", to-from, read)
						}
						from = to
					}
				}
				h.SetSpan(int64(span))
				resHash := h.Sum(nil)
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
				t.Fatal(err)
			}
		})
	}
}

// helper function that compares reference and optimised implementations on
// correctness
func testHasherCorrectness(h *bmt.Hasher, hasher bmt.BaseHasherFunc, data []byte, n, count int) (err error) {
	if len(data) < n {
		n = len(data)
	}
	exp, err := refHash(count, n, data)
	if err != nil {
		return err
	}
	got, err := syncHash(h, n, data)
	if err != nil {
		return err
	}
	if !bytes.Equal(got, exp) {
		return fmt.Errorf("wrong hash: expected %x, got %x", exp, got)
	}
	return err
}

// TestUseSyncAsOrdinaryHasher verifies that the bmt.Hasher can be used with the hash.Hash interface
func TestUseSyncAsOrdinaryHasher(t *testing.T) {
	pool := bmt.NewPool(bmt.NewConf(testHasher, testSegmentCount, testPoolSize))
	h := pool.Get()
	defer pool.Put(h)
	data := []byte("moodbytesmoodbytesmoodbytesmoodbytes")
	resHash, err := syncHash(h, 36, data)
	if err != nil {
		t.Fatal(err)
	}
	expHash, err := refHash(128, 36, data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expHash, resHash) {
		t.Fatalf("normalhash; expected %x, got %x", expHash, resHash)
	}
}
