// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"bytes"
	"context"
	"testing"
	"time"

	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/google/go-cmp/cmp"
)

// nolint:paralleltest
func TestReserveSampler(t *testing.T) {
	t.Parallel()

	const chunkCountPerPO = 10
	const maxPO = 10
	var chs []swarm.Chunk

	db := newTestDB(t, &Options{
		Capacity:        1000,
		ReserveCapacity: 1000,
	})

	timeVar := time.Now().UnixNano()

	for po := 0; po < maxPO; po++ {
		for i := 0; i < chunkCountPerPO; i++ {
			ch := generateTestRandomChunkAt(swarm.NewAddress(db.baseKey), po).WithBatch(0, 3, 2, false)
			// override stamp timestamp to be before the consensus timestamp
			ch = ch.WithStamp(postagetesting.MustNewStampWithTimestamp(uint64(timeVar) - 1))
			chs = append(chs, ch)
		}
	}

	_, err := db.Put(context.Background(), storage.ModePutSync, chs...)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("reserve size", reserveSizeTest(db, chunkCountPerPO*maxPO))

	var sample1 storage.Sample

	t.Run("reserve sample 1", func(t *testing.T) {
		sample, err := db.ReserveSample(context.TODO(), []byte("anchor"), 5, uint64(timeVar))
		if err != nil {
			t.Fatal(err)
		}
		if len(sample.Items) != sampleSize {
			t.Fatalf("incorrect no of sample items exp %d found %d", sampleSize, len(sample.Items))
		}
		for i := 0; i < len(sample.Items)-2; i++ {
			if bytes.Compare(sample.Items[i].Bytes(), sample.Items[i+1].Bytes()) != -1 {
				t.Fatalf("incorrect order of samples %+q", sample.Items)
			}
		}

		sample1 = sample
	})

	// We generate another 100 chunks. With these new chunks in the reserve, statistically
	// some of them should definitely make it to the sample based on lex ordering.
	for po := 0; po < maxPO; po++ {
		for i := 0; i < chunkCountPerPO; i++ {
			ch := generateTestRandomChunkAt(swarm.NewAddress(db.baseKey), po).WithBatch(0, 3, 2, false)
			// override stamp timestamp to be after the consensus timestamp
			ch = ch.WithStamp(postagetesting.MustNewStampWithTimestamp(uint64(timeVar) + 1))
			chs = append(chs, ch)
		}
	}

	_, err = db.Put(context.Background(), storage.ModePutSync, chs...)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("reserve size", reserveSizeTest(db, 2*chunkCountPerPO*maxPO))

	// Now we generate another sample with the older timestamp. This should give us
	// the exact same sample, ensuring that none of the later chunks were considered.
	t.Run("reserve sample 2", func(t *testing.T) {
		sample, err := db.ReserveSample(context.TODO(), []byte("anchor"), 5, uint64(timeVar))
		if err != nil {
			t.Fatal(err)
		}

		if !cmp.Equal(sample, sample1) {
			t.Fatalf("samples different (-want +have):\n%s", cmp.Diff(sample, sample1))
		}
	})
}
