// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package legacy

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bmt"
	"github.com/ethersphere/bmt/reference"
	"gitlab.com/nolash/go-mockbytes"
	"golang.org/x/crypto/sha3"
)

// the actual data length generated (could be longer than max datalength of the BMT)
const BufferSize = 4128

const (
	// segmentCount is the maximum number of segments of the underlying chunk
	// Should be equal to max-chunk-data-size / hash-size
	// Currently set to 128 == 4096 (default chunk size) / 32 (sha3.keccak256 size)
	SegmentCount = 128
)

var Counts = []int{1, 2, 3, 4, 5, 8, 9, 15, 16, 17, 32, 37, 42, 53, 63, 64, 65, 111, 127, 128}

var BenchmarkBMTResult []byte

// calculates the Keccak256 SHA3 hash of the data
func sha3hash(data ...[]byte) []byte {
	h := sha3.NewLegacyKeccak256()
	return doSum(h, nil, data...)
}

// tests if hasher responds with correct hash comparing the reference implementation return value
func TestHasherEmptyData(t *testing.T) {
	hasher := sha3.NewLegacyKeccak256
	var data []byte
	for _, count := range Counts {
		t.Run(fmt.Sprintf("%d_segments", count), func(t *testing.T) {
			pool := NewTreePool(hasher, count, PoolSize)
			defer pool.Drain(0)
			bmt := New(pool)
			rbmt := reference.NewRefHasher(hasher(), count)
			expHash, err := rbmt.Hash(data)
			if err != nil {
				t.Fatal(err)
			}
			resHash, err := syncHash(bmt, 0, data)
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
	g := mockbytes.New(1, mockbytes.MockTypeStandard)
	data, err := g.RandomBytes(BufferSize)
	if err != nil {
		t.Fatal(err)
	}
	hasher := sha3.NewLegacyKeccak256
	size := hasher().Size()

	for _, count := range Counts {
		t.Run(fmt.Sprintf("segments_%v", count), func(t *testing.T) {
			max := count * size
			var incr int
			capacity := 1
			pool := NewTreePool(hasher, count, capacity)
			defer pool.Drain(0)
			for n := 0; n <= max; n += incr {
				incr = 1 + rand.Intn(5)
				bmt := New(pool)
				err = testHasherCorrectness(bmt, hasher, data, n, count)
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// Tests that the BMT hasher can be synchronously reused with poolsizes 1 and PoolSize
func TestHasherReuse(t *testing.T) {
	t.Run(fmt.Sprintf("poolsize_%d", 1), func(t *testing.T) {
		testHasherReuse(1, t)
	})
	t.Run(fmt.Sprintf("poolsize_%d", PoolSize), func(t *testing.T) {
		testHasherReuse(PoolSize, t)
	})
}

// tests if bmt reuse is not corrupting result
func testHasherReuse(poolsize int, t *testing.T) {
	hasher := sha3.NewLegacyKeccak256
	pool := NewTreePool(hasher, SegmentCount, poolsize)
	defer pool.Drain(0)
	bmt := New(pool)

	for i := 0; i < 100; i++ {

		g := mockbytes.New(1, mockbytes.MockTypeStandard)
		data, err := g.RandomBytes(BufferSize)
		if err != nil {
			t.Fatal(err)
		}
		n := rand.Intn(bmt.Size())
		err = testHasherCorrectness(bmt, hasher, data, n, SegmentCount)
		if err != nil {
			t.Fatal(err)
		}
	}
}

// Tests if pool can be cleanly reused even in concurrent use by several hasher
func TestBMTConcurrentUse(t *testing.T) {
	hasher := sha3.NewLegacyKeccak256
	pool := NewTreePool(hasher, SegmentCount, PoolSize)
	defer pool.Drain(0)
	cycles := 100
	errc := make(chan error)

	for i := 0; i < cycles; i++ {
		go func() {
			bmt := New(pool)
			g := mockbytes.New(1, mockbytes.MockTypeStandard)
			data, _ := g.RandomBytes(BufferSize)
			n := rand.Intn(bmt.Size())
			errc <- testHasherCorrectness(bmt, hasher, data, n, 128)
		}()
	}
LOOP:
	for {
		select {
		case <-time.NewTimer(5 * time.Second).C:
			t.Fatal("timed out")
		case err := <-errc:
			if err != nil {
				t.Fatal(err)
			}
			cycles--
			if cycles == 0 {
				break LOOP
			}
		}
	}
}

// Tests BMT Hasher io.Writer interface is working correctly
// even multiple short random write buffers
func TestBMTWriterBuffers(t *testing.T) {
	hasher := sha3.NewLegacyKeccak256

	for _, count := range Counts {
		t.Run(fmt.Sprintf("%d_segments", count), func(t *testing.T) {
			errc := make(chan error)
			pool := NewTreePool(hasher, count, PoolSize)
			defer pool.Drain(0)
			n := count * 32
			bmt := New(pool)

			g := mockbytes.New(1, mockbytes.MockTypeStandard)
			data, err := g.RandomBytes(n)
			if err != nil {
				t.Fatal(err)
			}
			rbmt := reference.NewRefHasher(hasher(), count)
			refNoMetaHash, err := rbmt.Hash(data)
			if err != nil {
				t.Fatal(err)
			}
			h := hasher()
			_, err = h.Write(ZeroSpan)
			if err != nil {
				t.Fatal(err)
			}
			_, err = h.Write(refNoMetaHash)
			if err != nil {
				t.Fatal(err)
			}
			refHash := h.Sum(nil)
			expHash, err := syncHash(bmt, 0, data)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(expHash, refHash) {
				t.Fatalf("hash mismatch with reference. expected %x, got %x", refHash, expHash)
			}
			attempts := 10
			f := func() error {
				bmt := New(pool)
				bmt.Reset()
				var buflen int
				for offset := 0; offset < n; offset += buflen {
					buflen = rand.Intn(n-offset) + 1
					read, err := bmt.Write(data[offset : offset+buflen])
					if err != nil {
						return err
					}
					if read != buflen {
						return fmt.Errorf("incorrect read. expected %v bytes, got %v", buflen, read)
					}
				}
				err := bmt.SetSpan(0)
				if err != nil {
					t.Fatal(err)
				}
				hash := bmt.Sum(nil)
				if !bytes.Equal(hash, expHash) {
					return fmt.Errorf("hash mismatch. expected %x, got %x", hash, expHash)
				}
				return nil
			}

			for j := 0; j < attempts; j++ {
				go func() {
					errc <- f()
				}()
			}
			timeout := time.NewTimer(2 * time.Second)
			for {
				select {
				case err := <-errc:
					if err != nil {
						t.Fatal(err)
					}
					attempts--
					if attempts == 0 {
						return
					}
				case <-timeout.C:
					t.Fatalf("timeout")
				}
			}
		})
	}
}

// helper function that compares reference and optimised implementations on
// correctness
func testHasherCorrectness(bmt *Hasher, hasher BaseHasherFunc, d []byte, n, count int) (err error) {
	span := make([]byte, 8)
	if len(d) < n {
		n = len(d)
	}
	binary.LittleEndian.PutUint64(span, uint64(n))
	data := d[:n]
	rbmt := reference.NewRefHasher(hasher(), count)
	var exp []byte
	if n == 0 {
		exp = bmt.pool.zerohashes[bmt.pool.Depth]
	} else {
		r, err := rbmt.Hash(data)
		if err != nil {
			return err
		}
		exp = sha3hash(span, r)
	}
	got, err := syncHash(bmt, n, data)
	if err != nil {
		return err
	}
	if !bytes.Equal(got, exp) {
		return fmt.Errorf("wrong hash: expected %x, got %x", exp, got)
	}
	return err
}

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
	caps := []int{1, PoolSize}
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

	g := mockbytes.New(1, mockbytes.MockTypeStandard)
	data, err := g.RandomBytes(n)
	if err != nil {
		t.Fatal(err)
	}
	hasher := sha3.NewLegacyKeccak256
	h := hasher()

	t.ReportAllocs()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		doSum(h, nil, data)
	}
}

// benchmarks the minimum hashing time for a balanced (for simplicity) BMT
// by doing count/segmentsize parallel hashings of 2*segmentsize bytes
// doing it on n PoolSize each reusing the base hasher
// the premise is that this is the minimum computation needed for a BMT
// therefore this serves as a theoretical optimum for concurrent implementations
func benchmarkBMTBaseline(t *testing.B, n int) {
	hasher := sha3.NewLegacyKeccak256
	hashSize := hasher().Size()

	g := mockbytes.New(1, mockbytes.MockTypeStandard)
	data, err := g.RandomBytes(hashSize)
	if err != nil {
		t.Fatal(err)
	}

	t.ReportAllocs()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		count := int32((n-1)/hashSize + 1)
		wg := sync.WaitGroup{}
		wg.Add(PoolSize)
		var i int32
		for j := 0; j < PoolSize; j++ {
			go func() {
				defer wg.Done()
				h := hasher()
				for atomic.AddInt32(&i, 1) < count {
					doSum(h, nil, data)
				}
			}()
		}
		wg.Wait()
	}
}

// benchmarks BMT Hasher
func benchmarkBMT(t *testing.B, n int) {

	g := mockbytes.New(1, mockbytes.MockTypeStandard)
	data, err := g.RandomBytes(n)
	if err != nil {
		t.Fatal(err)
	}
	hasher := sha3.NewLegacyKeccak256
	pool := NewTreePool(hasher, SegmentCount, PoolSize)
	bmt := New(pool)
	var r []byte

	t.ReportAllocs()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		r, err = syncHash(bmt, 0, data)
		if err != nil {
			t.Fatal(err)
		}
	}
	BenchmarkBMTResult = r
}

// benchmarks 100 concurrent bmt hashes with pool capacity
func benchmarkPool(t *testing.B, poolsize, n int) {

	g := mockbytes.New(1, mockbytes.MockTypeStandard)
	data, err := g.RandomBytes(n)
	if err != nil {
		t.Fatal(err)
	}
	hasher := sha3.NewLegacyKeccak256
	pool := NewTreePool(hasher, SegmentCount, poolsize)
	cycles := 100

	t.ReportAllocs()
	t.ResetTimer()
	wg := sync.WaitGroup{}
	for i := 0; i < t.N; i++ {
		wg.Add(cycles)
		for j := 0; j < cycles; j++ {
			go func() {
				defer wg.Done()
				bmt := New(pool)
				_, _ = syncHash(bmt, 0, data)
			}()
		}
		wg.Wait()
	}
}

// benchmarks the reference hasher
func benchmarkRefHasher(t *testing.B, n int) {

	g := mockbytes.New(1, mockbytes.MockTypeStandard)
	data, err := g.RandomBytes(n)
	if err != nil {
		t.Fatal(err)
	}
	hasher := sha3.NewLegacyKeccak256
	rbmt := reference.NewRefHasher(hasher(), 128)

	t.ReportAllocs()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		_, err := rbmt.Hash(data)
		if err != nil {
			t.Fatal(err)
		}
	}
}

// Hash hashes the data and the span using the bmt hasher
func syncHash(h *Hasher, spanLength int, data []byte) ([]byte, error) {
	h.Reset()
	err := h.SetSpan(int64(spanLength))
	if err != nil {
		return nil, err
	}
	_, err = h.Write(data)
	if err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

// TestUseSyncAsOrdinaryHasher verifies that the bmt.Hasher can be used with the hash.Hash interface
func TestUseSyncAsOrdinaryHasher(t *testing.T) {
	hasher := sha3.NewLegacyKeccak256
	pool := NewTreePool(hasher, SegmentCount, PoolSize)
	bmt := New(pool)
	err := bmt.SetSpan(3)
	if err != nil {
		t.Fatal(err)
	}
	_, err = bmt.Write([]byte("foo"))
	if err != nil {
		t.Fatal(err)
	}
	res := bmt.Sum(nil)
	refh := reference.NewRefHasher(hasher(), 128)
	resh, err := refh.Hash([]byte("foo"))
	if err != nil {
		t.Fatal(err)
	}
	hsub := hasher()
	span := LengthToSpan(3)
	_, err = hsub.Write(span)
	if err != nil {
		t.Fatal(err)
	}

	_, err = hsub.Write(resh)
	if err != nil {
		t.Fatal(err)
	}
	refRes := hsub.Sum(nil)
	if !bytes.Equal(res, refRes) {
		t.Fatalf("normalhash; expected %x, got %x", refRes, res)
	}
}

func TestConformsToBMTInterface(t *testing.T) {
	func() bmt.Hash {
		return (New(nil))
	}()
}
