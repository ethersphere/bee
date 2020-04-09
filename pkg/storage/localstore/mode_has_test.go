// Copyright 2019 The go-ethereum Authors
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
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/ethersphere/swarm/chunk"
)

// TestHas validates that Has method is returning true for
// the stored chunk and false for one that is not stored.
func TestHas(t *testing.T) {
	db, cleanupFunc := newTestDB(t, nil)
	defer cleanupFunc()

	ch := generateTestRandomChunk()

	_, err := db.Put(context.Background(), chunk.ModePutUpload, ch)
	if err != nil {
		t.Fatal(err)
	}

	has, err := db.Has(context.Background(), ch.Address())
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Error("chunk not found")
	}

	missingChunk := generateTestRandomChunk()

	has, err = db.Has(context.Background(), missingChunk.Address())
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("unexpected chunk is found")
	}
}

// TestHasMulti validates that HasMulti method is returning correct boolean
// slice for stored chunks.
func TestHasMulti(t *testing.T) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for _, tc := range multiChunkTestCases {
		t.Run(tc.name, func(t *testing.T) {
			db, cleanupFunc := newTestDB(t, nil)
			defer cleanupFunc()

			chunks := generateTestRandomChunks(tc.count)
			want := make([]bool, tc.count)

			for i, ch := range chunks {
				if r.Intn(2) == 0 {
					// randomly exclude half of the chunks
					continue
				}
				_, err := db.Put(context.Background(), chunk.ModePutUpload, ch)
				if err != nil {
					t.Fatal(err)
				}
				want[i] = true
			}

			got, err := db.HasMulti(context.Background(), chunkAddresses(chunks)...)
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("got %v, want %v", got, want)
			}
		})
	}
}
