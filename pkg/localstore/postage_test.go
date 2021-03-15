package localstore

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var postageTestCases = []struct {
	mode    storage.ModePut
	count   int
	batches int
	depth   uint8
}{
	{storage.ModePutRequest, 1, 1, 8},
	{storage.ModePutRequest, 16, 2, 8},
	{storage.ModePutSync, 1, 1, 8},
	{storage.ModePutSync, 16, 2, 8},
	{storage.ModePutUpload, 1, 1, 8},
	{storage.ModePutUpload, 16, 2, 8},
	{storage.ModePutUpload, 16, 1, 8},
	{storage.ModePutUpload, 16, 1, 8},
}

func TestPutPostage(t *testing.T) {
	for _, tc := range postageTestCases {
		t.Run(fmt.Sprintf("mode:%v,count=%v,batches=%v,depth:%v", tc.mode, tc.count, tc.batches, tc.depth), func(t *testing.T) {
			db := newTestDB(t, nil)
			chunks := generateTestRandomChunks(tc.count)
			for i := tc.batches; i < tc.count; i++ {
				chunks[i].WithStamp(chunks[i-1].Stamp()).WithBatch(0, tc.depth)
			}
			t.Run("first put", func(t *testing.T) {
				for _, ch := range chunks {
					_, err := db.Put(context.Background(), tc.mode, ch)
					if err != nil {
						t.Fatal(err)
					}
				}
				newItemsCountTest(db.retrievalDataIndex, tc.count)(t)
				newItemsCountTest(db.postage.chunks, tc.count)(t)
				newItemsCountTest(db.postage.counts, tc.batches)(t)
			})

			t.Run("second put", func(t *testing.T) {
				for _, ch := range chunks {
					exists, err := db.Put(context.Background(), tc.mode, ch)
					if err != nil {
						t.Fatal(err)
					}
					if !exists[0] {
						t.Fatalf("expected chunk to exists in db")
					}
				}
				newItemsCountTest(db.retrievalDataIndex, tc.count)(t)
				newItemsCountTest(db.postage.chunks, tc.count)(t)
				newItemsCountTest(db.postage.counts, tc.batches)(t)
			})
		})
	}
}

func TestPutPostageOverissue(t *testing.T) {
	for _, mode := range []storage.ModePut{storage.ModePutRequest, storage.ModePutSync, storage.ModePutUpload} {
		for depth := 0; depth < int(swarm.MaxPO); depth++ {
			for oi := 0; oi <= depth; oi++ {
				t.Run(fmt.Sprintf("mode: %v,overissued depth: %v,batch depth:%v", mode, oi, depth), func(t *testing.T) {
					db := newTestDB(t, nil)
					// generate chunk with address matching oi bits of the overlay
					chunk := generateTestRandomChunkAt(swarm.NewAddress(db.baseKey), oi).WithBatch(0, uint8(depth))
					// extract postage stamp which will be reused
					stamp := chunk.Stamp()
					//
					head := binary.BigEndian.Uint32(chunk.Address().Bytes()[:4])
					order := depth - oi
					max := 1 << order
					seed := (head << oi) >> (32 - order)
					rem := (head << oi) >> oi
					prefix := head - rem
					chunks := make([]swarm.Chunk, max)
					for i := 0; i < max; i++ {
						_, err := db.Put(context.Background(), mode, chunk)
						if err != nil {
							t.Fatal(err)
						}
						chunks[i] = chunk
						chunk = generateTestRandomChunk()
						addr := chunk.Address().Bytes()
						balanced := ((seed + uint32(i+1)) % uint32(max)) << (32 - depth)
						random := (binary.BigEndian.Uint32(addr[:4]) << depth) >> depth
						head = prefix + balanced + random
						binary.BigEndian.PutUint32(addr[:4], head)
						chunk = swarm.NewChunk(swarm.NewAddress(addr), chunk.Data()).WithStamp(stamp).WithBatch(0, uint8(depth))
					}

					_, err := db.Put(context.Background(), mode, chunk)
					if !errors.Is(err, ErrBatchOverissued) {
						t.Fatalf("expected error %v, got %v", ErrBatchOverissued, err)
					}
					newItemsCountTest(db.retrievalDataIndex, max)(t)
					newItemsCountTest(db.postage.chunks, max)(t)
					newItemsCountTest(db.postage.counts, 1)(t)

					t.Run("second put", func(t *testing.T) {
						for _, ch := range chunks {
							exists, err := db.Put(context.Background(), mode, ch)
							if err != nil {
								t.Fatal(err)
							}
							if !exists[0] {
								t.Fatalf("expected chunk to exists in db")
							}
						}
						newItemsCountTest(db.retrievalDataIndex, max)(t)
						newItemsCountTest(db.postage.chunks, max)(t)
						newItemsCountTest(db.postage.counts, 1)(t)
					})
				})
			}
		}
	}
}
