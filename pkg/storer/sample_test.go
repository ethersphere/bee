// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"context"
	"math/rand"
	"testing"
	"time"

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
		for po := 0; po < maxPO; po++ {
			for i := 0; i < chunkCountPerPO; i++ {
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
			for i := 0; i < chunkCountPerPO; i++ {
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
