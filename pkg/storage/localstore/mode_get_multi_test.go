// Copyright 2019 The Swarm Authors
// This file is part of the Swarm library.
//
// The Swarm library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Swarm library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Swarm library. If not, see <http://www.gnu.org/licenses/>.

package localstore

import (
	"context"
	"reflect"
	"testing"

	"github.com/ethersphere/swarm/chunk"
)

// TestModeGetMulti stores chunks and validates that GetMulti
// is returning them correctly.
func TestModeGetMulti(t *testing.T) {
	const chunkCount = 10

	for _, mode := range []chunk.ModeGet{
		chunk.ModeGetRequest,
		chunk.ModeGetSync,
		chunk.ModeGetLookup,
		chunk.ModeGetPin,
	} {
		t.Run(mode.String(), func(t *testing.T) {
			db, cleanupFunc := newTestDB(t, nil)
			defer cleanupFunc()

			chunks := generateTestRandomChunks(chunkCount)

			_, err := db.Put(context.Background(), chunk.ModePutUpload, chunks...)
			if err != nil {
				t.Fatal(err)
			}

			if mode == chunk.ModeGetPin {
				// pin chunks so that it is not returned as not found by pinIndex
				for i, ch := range chunks {
					err := db.Set(context.Background(), chunk.ModeSetPin, ch.Address())
					if err != nil {
						t.Fatal(err)
					}
					chunks[i] = ch.WithPinCounter(1)
				}
			}

			addrs := chunkAddresses(chunks)

			got, err := db.GetMulti(context.Background(), mode, addrs...)
			if err != nil {
				t.Fatal(err)
			}

			for i := 0; i < chunkCount; i++ {
				if !reflect.DeepEqual(got[i], chunks[i]) {
					t.Errorf("got %v chunk %v, want %v", i, got[i], chunks[i])
				}
			}

			missingChunk := generateTestRandomChunk()

			want := chunk.ErrChunkNotFound
			_, err = db.GetMulti(context.Background(), mode, append(addrs, missingChunk.Address())...)
			if err != want {
				t.Errorf("got error %v, want %v", err, want)
			}
		})
	}
}
