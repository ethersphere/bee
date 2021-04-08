package localstore

import (
	"context"
	"fmt"
	"testing"

	"github.com/ethersphere/bee/pkg/storage"
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
				newItemsCountTest(db.postageChunksIndex, tc.count)(t)
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
				newItemsCountTest(db.postageChunksIndex, tc.count)(t)
			})
		})
	}
}
