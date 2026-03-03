// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/postage"

	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	chunk "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/google/go-cmp/cmp"
)

func TestReserveSampler(t *testing.T) {
	const chunkCountPerPO = 10
	const maxPO = 10

	randChunks := func(baseAddr swarm.Address, timeVar uint64) []swarm.Chunk {
		var chs []swarm.Chunk
		for po := range maxPO {
			for range chunkCountPerPO {
				ch := chunk.GenerateValidRandomChunkAt(t, baseAddr, po).WithBatch(3, 2, false)
				if rand.Intn(2) == 0 { // 50% chance to wrap CAC into SOC
					ch = chunk.GenerateTestRandomSoChunk(t, ch)
				}

				// override stamp timestamp to be before the consensus timestamp
				ch = ch.WithStamp(postagetesting.MustNewStampWithTimestamp(timeVar))
				chs = append(chs, ch)
			}
		}
		return chs
	}

	testF := func(t *testing.T, baseAddr swarm.Address, st *storer.DB) {
		t.Helper()

		timeVar := uint64(time.Now().UnixNano())
		chs := randChunks(baseAddr, timeVar-1)

		putter := st.ReservePutter()
		for _, ch := range chs {
			err := putter.Put(context.Background(), ch)
			if err != nil {
				t.Fatal(err)
			}
		}

		t.Run("reserve size", reserveSizeTest(st.Reserve(), chunkCountPerPO*maxPO))

		var sample1 storer.Sample

		var (
			radius uint8 = 5
			anchor       = swarm.RandAddressAt(t, baseAddr, int(radius)).Bytes()
		)

		t.Run("reserve sample 1", func(t *testing.T) {
			sample, err := st.ReserveSample(context.TODO(), anchor, radius, timeVar, nil)
			if err != nil {
				t.Fatal(err)
			}

			assertValidSample(t, sample, radius, anchor)
			assertSampleNoErrors(t, sample)

			if sample.Stats.NewIgnored != 0 {
				t.Fatalf("sample should not have ignored chunks")
			}

			sample1 = sample
		})

		// We generate another 100 chunks. With these new chunks in the reserve, statistically
		// some of them should definitely make it to the sample based on lex ordering.
		chs = randChunks(baseAddr, timeVar+1)
		putter = st.ReservePutter()
		for _, ch := range chs {
			err := putter.Put(context.Background(), ch)
			if err != nil {
				t.Fatal(err)
			}
		}

		time.Sleep(time.Second)

		t.Run("reserve size", reserveSizeTest(st.Reserve(), 2*chunkCountPerPO*maxPO))

		// Now we generate another sample with the older timestamp. This should give us
		// the exact same sample, ensuring that none of the later chunks were considered.
		t.Run("reserve sample 2", func(t *testing.T) {
			sample, err := st.ReserveSample(context.TODO(), anchor, 5, timeVar, nil)
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(sample.Items, sample1.Items, cmp.AllowUnexported(postage.Stamp{})); diff != "" {
				t.Fatalf("samples different (-want +have):\n%s", diff)
			}

			if sample.Stats.NewIgnored == 0 {
				t.Fatalf("sample should have some ignored chunks")
			}

			assertSampleNoErrors(t, sample)
		})
	}

	t.Run("disk", func(t *testing.T) {
		t.Parallel()
		baseAddr := swarm.RandAddress(t)
		opts := dbTestOps(baseAddr, 1000, nil, nil, time.Second)
		opts.ValidStamp = func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }

		storer, err := diskStorer(t, opts)()
		if err != nil {
			t.Fatal(err)
		}
		testF(t, baseAddr, storer)
	})
	t.Run("mem", func(t *testing.T) {
		t.Parallel()
		baseAddr := swarm.RandAddress(t)
		opts := dbTestOps(baseAddr, 1000, nil, nil, time.Second)
		opts.ValidStamp = func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }

		storer, err := memStorer(t, opts)()
		if err != nil {
			t.Fatal(err)
		}
		testF(t, baseAddr, storer)
	})
}

func TestReserveSamplerSisterNeighborhood(t *testing.T) {
	t.Parallel()

	const (
		chunkCountPerPO             = 64
		maxPO                       = 6
		committedDepth        uint8 = 5
		doubling              uint8 = 2
		depthOfResponsibility uint8 = committedDepth - doubling
	)

	randChunks := func(baseAddr swarm.Address, startingRadius int, timeVar uint64) []swarm.Chunk {
		var chs []swarm.Chunk
		for po := startingRadius; po < maxPO; po++ {
			for range chunkCountPerPO {
				ch := chunk.GenerateValidRandomChunkAt(t, baseAddr, po).WithBatch(3, 2, false)
				if rand.Intn(2) == 0 { // 50% chance to wrap CAC into SOC
					ch = chunk.GenerateTestRandomSoChunk(t, ch)
				}

				// override stamp timestamp to be before the consensus timestamp
				ch = ch.WithStamp(postagetesting.MustNewStampWithTimestamp(timeVar))
				chs = append(chs, ch)
			}
		}
		return chs
	}

	testF := func(t *testing.T, baseAddr swarm.Address, st *storer.DB) {
		t.Helper()

		count := 0
		// local neighborhood
		timeVar := uint64(time.Now().UnixNano())
		chs := randChunks(baseAddr, int(committedDepth), timeVar)
		putter := st.ReservePutter()
		for _, ch := range chs {
			err := putter.Put(context.Background(), ch)
			if err != nil {
				t.Fatal(err)
			}
		}
		count += len(chs)

		sisterAnchor := swarm.RandAddressAt(t, baseAddr, int(depthOfResponsibility))

		// chunks belonging to the sister neighborhood
		chs = randChunks(sisterAnchor, int(committedDepth), timeVar)
		putter = st.ReservePutter()
		for _, ch := range chs {
			err := putter.Put(context.Background(), ch)
			if err != nil {
				t.Fatal(err)
			}
		}
		count += len(chs)

		t.Run("reserve size", reserveSizeTest(st.Reserve(), count))

		t.Run("reserve sample", func(t *testing.T) {
			sample, err := st.ReserveSample(context.TODO(), sisterAnchor.Bytes(), doubling, timeVar, nil)
			if err != nil {
				t.Fatal(err)
			}

			assertValidSample(t, sample, doubling, baseAddr.Bytes())
			assertSampleNoErrors(t, sample)

			if sample.Stats.NewIgnored != 0 {
				t.Fatalf("sample should not have ignored chunks")
			}
		})

		t.Run("reserve sample 2", func(t *testing.T) {
			sample, err := st.ReserveSample(context.TODO(), sisterAnchor.Bytes(), committedDepth, timeVar, nil)
			if err != nil {
				t.Fatal(err)
			}

			assertValidSample(t, sample, depthOfResponsibility, baseAddr.Bytes())
			assertSampleNoErrors(t, sample)

			for _, s := range sample.Items {
				if got := swarm.Proximity(s.ChunkAddress.Bytes(), baseAddr.Bytes()); got != depthOfResponsibility {
					t.Fatalf("proximity must be exactly %d, got %d", depthOfResponsibility, got)
				}
			}

			if sample.Stats.NewIgnored != 0 {
				t.Fatalf("sample should not have ignored chunks")
			}
		})
	}

	t.Run("disk", func(t *testing.T) {
		t.Parallel()
		baseAddr := swarm.RandAddress(t)
		opts := dbTestOps(baseAddr, 1000, nil, nil, time.Second)
		opts.ValidStamp = func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }
		opts.ReserveCapacityDoubling = 2

		storer, err := diskStorer(t, opts)()
		if err != nil {
			t.Fatal(err)
		}
		testF(t, baseAddr, storer)
	})
	t.Run("mem", func(t *testing.T) {
		t.Parallel()
		baseAddr := swarm.RandAddress(t)
		opts := dbTestOps(baseAddr, 1000, nil, nil, time.Second)
		opts.ValidStamp = func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }
		opts.ReserveCapacityDoubling = 2

		storer, err := memStorer(t, opts)()
		if err != nil {
			t.Fatal(err)
		}
		testF(t, baseAddr, storer)
	})
}

func TestRandSample(t *testing.T) {
	t.Parallel()

	sample := storer.RandSample(t, nil)
	assertValidSample(t, sample, 0, nil)
}

func assertValidSample(t *testing.T, sample storer.Sample, minRadius uint8, anchor []byte) {
	t.Helper()

	// Assert that sample size is exactly storer.SampleSize
	if len(sample.Items) != storer.SampleSize {
		t.Fatalf("incorrect no of sample items, exp %d found %d", storer.SampleSize, len(sample.Items))
	}

	// Assert that sample item has all fields set
	assertSampleItem := func(item storer.SampleItem, i int) {
		if !item.TransformedAddress.IsValidNonEmpty() {
			t.Fatalf("sample item [%d]: transformed address should be set", i)
		}
		if !item.ChunkAddress.IsValidNonEmpty() {
			t.Fatalf("sample item [%d]: chunk address should be set", i)
		}
		if item.ChunkData == nil {
			t.Fatalf("sample item [%d]: chunk data should be set", i)
		}
		if item.Stamp == nil {
			t.Fatalf("sample item [%d]: stamp should be set", i)
		}
		if got := swarm.Proximity(item.ChunkAddress.Bytes(), anchor); got < minRadius {
			t.Fatalf("sample item [%d]: chunk should have proximity %d with the anchor, got %d", i, minRadius, got)
		}
	}
	for i, item := range sample.Items {
		assertSampleItem(item, i)
	}

	// Assert that transformed addresses are in ascending order
	for i := 0; i < len(sample.Items)-1; i++ {
		if sample.Items[i].TransformedAddress.Compare(sample.Items[i+1].TransformedAddress) != -1 {
			t.Fatalf("incorrect order of samples")
		}
	}
}

// TestSampleVectorCAC is a deterministic test vector that verifies the chunk
// address and transformed address produced by MakeSampleUsingChunks for a
// single hardcoded CAC chunk and anchor. It guards against regressions in the
// BMT hashing or sampling pipeline.
func TestSampleVectorCAC(t *testing.T) {
	t.Parallel()

	// Chunk content: 4096 bytes with repeating pattern i%256.
	chunkContent := make([]byte, swarm.ChunkSize)
	for i := range chunkContent {
		chunkContent[i] = byte(i % 256)
	}

	ch, err := cac.New(chunkContent)
	if err != nil {
		t.Fatal(err)
	}

	// Attach a hardcoded (but otherwise irrelevant) stamp so that
	// MakeSampleUsingChunks can read ch.Stamp() without panicking.
	batchID := make([]byte, 32)
	for i := range batchID {
		batchID[i] = byte(i + 1)
	}
	sig := make([]byte, 65)
	for i := range sig {
		sig[i] = byte(i + 1)
	}
	ch = ch.WithStamp(postage.NewStamp(batchID, make([]byte, 8), make([]byte, 8), sig))

	// Anchor: exactly 32 bytes, constant across runs.
	anchor := []byte("swarm-test-anchor-deterministic!")

	sample, err := storer.MakeSampleUsingChunks([]swarm.Chunk{ch}, anchor)
	if err != nil {
		t.Fatal(err)
	}

	if len(sample.Items) != 1 {
		t.Fatalf("expected 1 sample item, got %d", len(sample.Items))
	}

	item := sample.Items[0]

	const (
		wantChunkAddr       = "902406053a7a2f3a17f16097e1d0b4b6a4abeae6b84968f5503ae621f9522e16"
		wantTransformedAddr = "9dee91d1ed794460474ffc942996bd713176731db4581a3c6470fe9862905a60"
	)

	if got := item.ChunkAddress.String(); got != wantChunkAddr {
		t.Errorf("chunk address mismatch:\n got:  %s\n want: %s", got, wantChunkAddr)
	}
	if got := item.TransformedAddress.String(); got != wantTransformedAddr {
		t.Errorf("transformed address mismatch:\n got:  %s\n want: %s", got, wantTransformedAddr)
	}
}

func assertSampleNoErrors(t *testing.T, sample storer.Sample) {
	t.Helper()

	if sample.Stats.ChunkLoadFailed != 0 {
		t.Fatalf("got unexpected failed chunk loads")
	}
	if sample.Stats.RogueChunk != 0 {
		t.Fatalf("got unexpected rogue chunks")
	}
	if sample.Stats.StampLoadFailed != 0 {
		t.Fatalf("got unexpected failed stamp loads")
	}
	if sample.Stats.InvalidStamp != 0 {
		t.Fatalf("got unexpected invalid stamps")
	}
}

// Benchmark results:
// goos: linux
// goarch: amd64
// pkg: github.com/ethersphere/bee/v2/pkg/storer
// cpu: Intel(R) Core(TM) Ultra 7 165U
// BenchmarkCachePutter-14        	  473118	      2149 ns/op	    1184 B/op	      24 allocs/op
// BenchmarkReservePutter-14      	   48109	     29760 ns/op	   12379 B/op	     141 allocs/op
// BenchmarkReserveSample1k-14    	     100	  12392598 ns/op	 9364970 B/op	  161383 allocs/op
// BenchmarkSampleHashing/chunks=1000-14         	       9	 127425952 ns/op	  32.14 MB/s	69386109 B/op	  814005 allocs/op
// BenchmarkSampleHashing/chunks=10000-14        	       1	1241432669 ns/op	  32.99 MB/s	693843032 B/op	 8140005 allocs/op
// PASS
// ok  	github.com/ethersphere/bee/v2/pkg/storer	34.319s

// BenchmarkReserveSample measures the end-to-end time of the ReserveSample
// method, including DB iteration, chunk loading, stamp validation, and sample
// assembly.
func BenchmarkReserveSample1k(b *testing.B) {
	const chunkCountPerPO = 100
	const maxPO = 10

	baseAddr := swarm.RandAddress(b)
	opts := dbTestOps(baseAddr, 5000, nil, nil, time.Second)
	opts.ValidStamp = func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }

	st, err := diskStorer(b, opts)()
	if err != nil {
		b.Fatal(err)
	}

	timeVar := uint64(time.Now().UnixNano())

	putter := st.ReservePutter()
	for po := range maxPO {
		for range chunkCountPerPO {
			ch := chunk.GenerateValidRandomChunkAt(b, baseAddr, po).WithBatch(3, 2, false)
			ch = ch.WithStamp(postagetesting.MustNewStampWithTimestamp(timeVar - 1))
			if err := putter.Put(context.Background(), ch); err != nil {
				b.Fatal(err)
			}
		}
	}

	var (
		radius uint8 = 5
		anchor       = swarm.RandAddressAt(b, baseAddr, int(radius)).Bytes()
	)

	b.ResetTimer()

	for range b.N {
		_, err := st.ReserveSample(context.TODO(), anchor, radius, timeVar, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSampleHashing measures the time taken by MakeSampleUsingChunks to
// hash a fixed set of CAC chunks.
func BenchmarkSampleHashing(b *testing.B) {
	anchor := []byte("swarm-test-anchor-deterministic!")

	// Shared zero-value stamp: its contents don't affect hash computation.
	stamp := postage.NewStamp(make([]byte, 32), make([]byte, 8), make([]byte, 8), make([]byte, 65))

	for _, count := range []int{1_000, 10_000} {
		b.Run(fmt.Sprintf("chunks=%d", count), func(b *testing.B) {
			// Build chunks once outside the measured loop.
			// Content is derived deterministically from the chunk index so
			// that every run produces the same set of chunk addresses.
			chunks := make([]swarm.Chunk, count)
			content := make([]byte, swarm.ChunkSize)
			for i := range chunks {
				for j := range content {
					content[j] = byte(i + j)
				}
				ch, err := cac.New(content)
				if err != nil {
					b.Fatal(err)
				}
				chunks[i] = ch.WithStamp(stamp)
			}

			// Report throughput so the output shows MB/s as well as ns/op.
			b.SetBytes(int64(count) * swarm.ChunkSize)
			b.ResetTimer()

			for range b.N {
				if _, err := storer.MakeSampleUsingChunks(chunks, anchor); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
