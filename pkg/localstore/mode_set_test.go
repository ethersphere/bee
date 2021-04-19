// Copyright 2018 The go-ethereum Authors
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

package localstore

import (
	"context"
	"errors"
	"testing"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/syndtr/goleveldb/leveldb"
)

// TestModeSetRemove validates ModeSetRemove index values on the provided DB.
func TestModeSetRemove(t *testing.T) {
	for _, tc := range multiChunkTestCases {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t, nil)

			chunks := generateTestRandomChunks(tc.count)

			_, err := db.Put(context.Background(), storage.ModePutUpload, chunks...)
			if err != nil {
				t.Fatal(err)
			}

			err = db.Set(context.Background(), storage.ModeSetRemove, chunkAddresses(chunks)...)
			if err != nil {
				t.Fatal(err)
			}

			t.Run("retrieve indexes", func(t *testing.T) {
				for _, ch := range chunks {
					wantErr := leveldb.ErrNotFound
					_, err := db.retrievalDataIndex.Get(addressToItem(ch.Address()))
					if !errors.Is(err, wantErr) {
						t.Errorf("got error %v, want %v", err, wantErr)
					}

					// access index should not be set
					_, err = db.retrievalAccessIndex.Get(addressToItem(ch.Address()))
					if !errors.Is(err, wantErr) {
						t.Errorf("got error %v, want %v", err, wantErr)
					}
				}

				t.Run("retrieve data index count", newItemsCountTest(db.retrievalDataIndex, 0))

				t.Run("retrieve access index count", newItemsCountTest(db.retrievalAccessIndex, 0))
			})

			for _, ch := range chunks {
				newPullIndexTest(db, ch, 0, leveldb.ErrNotFound)(t)
			}

			t.Run("pull index count", newItemsCountTest(db.pullIndex, 0))

			t.Run("gc index count", newItemsCountTest(db.gcIndex, 0))

			t.Run("gc size", newIndexGCSizeTest(db))
		})
	}
}

// TestModeSetRemove_WithSync validates ModeSetRemove index values on the provided DB
// with the syncing flow for a reserved chunk that has been marked for removal.
func TestModeSetRemove_WithSync(t *testing.T) {
	for _, tc := range multiChunkTestCases {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t, nil)
			var chs []swarm.Chunk
			for i := 0; i < tc.count; i++ {
				ch := generateTestRandomChunkAt(swarm.NewAddress(db.baseKey), 2).WithBatch(2, 3)
				err := db.UnreserveBatch(ch.Stamp().BatchID(), 2)
				if err != nil {
					t.Fatal(err)
				}
				_, err = db.Put(context.Background(), storage.ModePutUpload, ch)
				if err != nil {
					t.Fatal(err)
				}
				err = db.Set(context.Background(), storage.ModeSetSync, ch.Address())
				if err != nil {
					t.Fatal(err)
				}

				chs = append(chs, ch)
			}

			err := db.Set(context.Background(), storage.ModeSetRemove, chunkAddresses(chs)...)
			if err != nil {
				t.Fatal(err)
			}

			t.Run("retrieve indexes", func(t *testing.T) {
				for _, ch := range chs {
					wantErr := leveldb.ErrNotFound
					_, err := db.retrievalDataIndex.Get(addressToItem(ch.Address()))
					if !errors.Is(err, wantErr) {
						t.Errorf("got error %v, want %v", err, wantErr)
					}

					// access index should not be set
					_, err = db.retrievalAccessIndex.Get(addressToItem(ch.Address()))
					if !errors.Is(err, wantErr) {
						t.Errorf("got error %v, want %v", err, wantErr)
					}
				}

				t.Run("retrieve data index count", newItemsCountTest(db.retrievalDataIndex, 0))

				t.Run("retrieve access index count", newItemsCountTest(db.retrievalAccessIndex, 0))
			})

			for _, ch := range chs {
				newPullIndexTest(db, ch, 0, leveldb.ErrNotFound)(t)
			}
			t.Run("postage chunks index count", newItemsCountTest(db.postageChunksIndex, 0))

			t.Run("postage index index count", newItemsCountTest(db.postageIndexIndex, 0))

			t.Run("pull index count", newItemsCountTest(db.pullIndex, 0))

			t.Run("gc index count", newItemsCountTest(db.gcIndex, 0))

			t.Run("gc size", newIndexGCSizeTest(db))
		})
	}
}
