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
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/syndtr/goleveldb/leveldb"
)

// TestModePutRequest validates ModePutRequest index values on the provided DB.
func TestModePutRequest(t *testing.T) {
	for _, tc := range multiChunkTestCases {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t, nil)

			chunks := generateTestRandomChunks(tc.count)

			// keep the record when the chunk is stored
			var storeTimestamp int64

			t.Run("first put", func(t *testing.T) {
				wantTimestamp := time.Now().UTC().UnixNano()
				defer setNow(func() (t int64) {
					return wantTimestamp
				})()

				storeTimestamp = wantTimestamp

				_, err := db.Put(context.Background(), storage.ModePutRequest, chunks...)
				if err != nil {
					t.Fatal(err)
				}

				for _, ch := range chunks {
					newRetrieveIndexesTestWithAccess(db, ch, wantTimestamp, wantTimestamp)(t)
				}

				newItemsCountTest(db.gcIndex, tc.count)(t)
				newIndexGCSizeTest(db)(t)
			})

			t.Run("second put", func(t *testing.T) {
				wantTimestamp := time.Now().UTC().UnixNano()
				defer setNow(func() (t int64) {
					return wantTimestamp
				})()

				_, err := db.Put(context.Background(), storage.ModePutRequest, chunks...)
				if err != nil {
					t.Fatal(err)
				}

				for _, ch := range chunks {
					newRetrieveIndexesTestWithAccess(db, ch, storeTimestamp, storeTimestamp)(t)
				}

				newItemsCountTest(db.gcIndex, tc.count)(t)
				newIndexGCSizeTest(db)(t)
			})
		})
	}
}

// TestModePutRequestPin validates ModePutRequestPin index values on the provided DB.
func TestModePutRequestPin(t *testing.T) {
	for _, tc := range multiChunkTestCases {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t, nil)

			chunks := generateTestRandomChunks(tc.count)

			wantTimestamp := time.Now().UTC().UnixNano()
			defer setNow(func() (t int64) {
				return wantTimestamp
			})()

			_, err := db.Put(context.Background(), storage.ModePutRequestPin, chunks...)
			if err != nil {
				t.Fatal(err)
			}

			for _, ch := range chunks {
				newRetrieveIndexesTestWithAccess(db, ch, wantTimestamp, wantTimestamp)(t)
				newPinIndexTest(db, ch, nil)(t)
			}

			newItemsCountTest(db.gcIndex, tc.count)(t)
		})
	}
}

// TestModePutSync validates ModePutSync index values on the provided DB.
func TestModePutSync(t *testing.T) {
	for _, tc := range multiChunkTestCases {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t, nil)

			wantTimestamp := time.Now().UTC().UnixNano()
			defer setNow(func() (t int64) {
				return wantTimestamp
			})()

			chunks := generateTestRandomChunks(tc.count)

			_, err := db.Put(context.Background(), storage.ModePutSync, chunks...)
			if err != nil {
				t.Fatal(err)
			}

			binIDs := make(map[uint8]uint64)

			for _, ch := range chunks {
				po := db.po(ch.Address())
				binIDs[po]++

				newRetrieveIndexesTestWithAccess(db, ch, wantTimestamp, wantTimestamp)(t)
				newPullIndexTest(db, ch, binIDs[po], nil)(t)
				newPinIndexTest(db, ch, leveldb.ErrNotFound)(t)
				newItemsCountTest(db.gcIndex, tc.count)(t)
				newIndexGCSizeTest(db)(t)
			}
			newItemsCountTest(db.gcIndex, tc.count)(t)
			newIndexGCSizeTest(db)(t)
		})
	}
}

// TestModePutUpload validates ModePutUpload index values on the provided DB.
func TestModePutUpload(t *testing.T) {
	for _, tc := range multiChunkTestCases {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t, nil)

			wantTimestamp := time.Now().UTC().UnixNano()
			defer setNow(func() (t int64) {
				return wantTimestamp
			})()

			chunks := generateTestRandomChunks(tc.count)

			_, err := db.Put(context.Background(), storage.ModePutUpload, chunks...)
			if err != nil {
				t.Fatal(err)
			}

			binIDs := make(map[uint8]uint64)

			for _, ch := range chunks {
				po := db.po(ch.Address())
				binIDs[po]++

				newRetrieveIndexesTest(db, ch, wantTimestamp, 0)(t)
				newPullIndexTest(db, ch, binIDs[po], nil)(t)
				newPushIndexTest(db, ch, wantTimestamp, nil)(t)
				newPinIndexTest(db, ch, leveldb.ErrNotFound)(t)
			}
		})
	}
}

// TestModePutUploadPin validates ModePutUploadPin index values on the provided DB.
func TestModePutUploadPin(t *testing.T) {
	for _, tc := range multiChunkTestCases {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t, nil)

			wantTimestamp := time.Now().UTC().UnixNano()
			defer setNow(func() (t int64) {
				return wantTimestamp
			})()

			chunks := generateTestRandomChunks(tc.count)

			_, err := db.Put(context.Background(), storage.ModePutUploadPin, chunks...)
			if err != nil {
				t.Fatal(err)
			}

			binIDs := make(map[uint8]uint64)

			for _, ch := range chunks {
				po := db.po(ch.Address())
				binIDs[po]++

				newRetrieveIndexesTest(db, ch, wantTimestamp, 0)(t)
				newPullIndexTest(db, ch, binIDs[po], nil)(t)
				newPushIndexTest(db, ch, wantTimestamp, nil)(t)
				newPinIndexTest(db, ch, nil)(t)
			}
		})
	}
}

// TestModePutUpload_parallel uploads chunks in parallel
// and validates if all chunks can be retrieved with correct data.
func TestModePutUpload_parallel(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count int
	}{
		{
			name:  "one",
			count: 1,
		},
		{
			name:  "two",
			count: 2,
		},
		{
			name:  "eight",
			count: 8,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t, nil)

			uploadsCount := 100
			workerCount := 100

			chunksChan := make(chan []swarm.Chunk)
			errChan := make(chan error)
			doneChan := make(chan struct{})
			defer close(doneChan)

			// start uploader workers
			for i := 0; i < workerCount; i++ {
				go func(i int) {
					for {
						select {
						case chunks, ok := <-chunksChan:
							if !ok {
								return
							}
							_, err := db.Put(context.Background(), storage.ModePutUpload, chunks...)
							select {
							case errChan <- err:
							case <-doneChan:
							}
						case <-doneChan:
							return
						}
					}
				}(i)
			}

			chunks := make([]swarm.Chunk, 0)
			var chunksMu sync.Mutex

			// send chunks to workers
			go func() {
				for i := 0; i < uploadsCount; i++ {
					chs := generateTestRandomChunks(tc.count)
					select {
					case chunksChan <- chs:
					case <-doneChan:
						return
					}
					chunksMu.Lock()
					chunks = append(chunks, chs...)
					chunksMu.Unlock()
				}

				close(chunksChan)
			}()

			// validate every error from workers
			for i := 0; i < uploadsCount; i++ {
				err := <-errChan
				if err != nil {
					t.Fatal(err)
				}
			}

			// get every chunk and validate its data
			chunksMu.Lock()
			defer chunksMu.Unlock()
			for _, ch := range chunks {
				got, err := db.Get(context.Background(), storage.ModeGetRequest, ch.Address())
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got.Data(), ch.Data()) {
					t.Fatalf("got chunk %s data %x, want %x", ch.Address(), got.Data(), ch.Data())
				}
			}
		})
	}
}

// TestModePut_sameChunk puts the same chunk multiple times
// and validates that all relevant indexes have only one item
// in them.
func TestModePut_sameChunk(t *testing.T) {
	for _, tc := range multiChunkTestCases {
		t.Run(tc.name, func(t *testing.T) {
			chunks := generateTestRandomChunks(tc.count)

			for _, tcn := range []struct {
				name      string
				mode      storage.ModePut
				pullIndex bool
				pushIndex bool
			}{
				{
					name:      "ModePutRequest",
					mode:      storage.ModePutRequest,
					pullIndex: false,
					pushIndex: false,
				},
				{
					name:      "ModePutUpload",
					mode:      storage.ModePutUpload,
					pullIndex: true,
					pushIndex: true,
				},
				{
					name:      "ModePutSync",
					mode:      storage.ModePutSync,
					pullIndex: true,
					pushIndex: false,
				},
			} {
				t.Run(tcn.name, func(t *testing.T) {
					db := newTestDB(t, nil)

					for i := 0; i < 10; i++ {
						exist, err := db.Put(context.Background(), tcn.mode, chunks...)
						if err != nil {
							t.Fatal(err)
						}
						for _, exists := range exist {
							switch exists {
							case false:
								if i != 0 {
									t.Fatal("should not exist only on first Put")
								}
							case true:
								if i == 0 {
									t.Fatal("should exist on all cases other than the first one")
								}
							}
						}

						count := func(b bool) (c int) {
							if b {
								return tc.count
							}
							return 0
						}

						newItemsCountTest(db.retrievalDataIndex, tc.count)(t)
						newItemsCountTest(db.pullIndex, count(tcn.pullIndex))(t)
						newItemsCountTest(db.pushIndex, count(tcn.pushIndex))(t)
					}
				})
			}
		})
	}
}

// TestPutDuplicateChunks validates the expected behaviour for
// passing duplicate chunks to the Put method.
func TestPutDuplicateChunks(t *testing.T) {
	for _, mode := range []storage.ModePut{
		storage.ModePutUpload,
		storage.ModePutRequest,
		storage.ModePutSync,
	} {
		t.Run(mode.String(), func(t *testing.T) {
			db := newTestDB(t, nil)

			ch := generateTestRandomChunk()

			exist, err := db.Put(context.Background(), mode, ch, ch)
			if err != nil {
				t.Fatal(err)
			}
			if exist[0] {
				t.Error("first chunk should not exist")
			}
			if !exist[1] {
				t.Error("second chunk should exist")
			}

			newItemsCountTest(db.retrievalDataIndex, 1)(t)

			got, err := db.Get(context.Background(), storage.ModeGetLookup, ch.Address())
			if err != nil {
				t.Fatal(err)
			}
			if !got.Address().Equal(ch.Address()) {
				t.Errorf("got chunk address %s, want %s", got.Address(), ch.Address())
			}
		})
	}
}

// BenchmarkPutUpload runs a series of benchmarks that upload
// a specific number of chunks in parallel.
//
// Measurements on MacBook Pro (Retina, 15-inch, Mid 2014)
//
// # go test -benchmem -run=none github.com/ethersphere/swarm/storage/localstore -bench BenchmarkPutUpload -v
//
// goos: darwin
// goarch: amd64
// pkg: github.com/ethersphere/swarm/storage/localstore
// BenchmarkPutUpload/count_100_parallel_1-8         	     300	   5107704 ns/op	 2081461 B/op	    2374 allocs/op
// BenchmarkPutUpload/count_100_parallel_2-8         	     300	   5411742 ns/op	 2081608 B/op	    2364 allocs/op
// BenchmarkPutUpload/count_100_parallel_4-8         	     500	   3704964 ns/op	 2081696 B/op	    2324 allocs/op
// BenchmarkPutUpload/count_100_parallel_8-8         	     500	   2932663 ns/op	 2082594 B/op	    2295 allocs/op
// BenchmarkPutUpload/count_100_parallel_16-8        	     500	   3117157 ns/op	 2085438 B/op	    2282 allocs/op
// BenchmarkPutUpload/count_100_parallel_32-8        	     500	   3449122 ns/op	 2089721 B/op	    2286 allocs/op
// BenchmarkPutUpload/count_1000_parallel_1-8        	      20	  79784470 ns/op	25211240 B/op	   23225 allocs/op
// BenchmarkPutUpload/count_1000_parallel_2-8        	      20	  75422164 ns/op	25210730 B/op	   23187 allocs/op
// BenchmarkPutUpload/count_1000_parallel_4-8        	      20	  70698378 ns/op	25206522 B/op	   22692 allocs/op
// BenchmarkPutUpload/count_1000_parallel_8-8        	      20	  71285528 ns/op	25213436 B/op	   22345 allocs/op
// BenchmarkPutUpload/count_1000_parallel_16-8       	      20	  71301826 ns/op	25205040 B/op	   22090 allocs/op
// BenchmarkPutUpload/count_1000_parallel_32-8       	      30	  57713506 ns/op	25219781 B/op	   21848 allocs/op
// BenchmarkPutUpload/count_10000_parallel_1-8       	       2	 656719345 ns/op	216792908 B/op	  248940 allocs/op
// BenchmarkPutUpload/count_10000_parallel_2-8       	       2	 646301962 ns/op	216730800 B/op	  248270 allocs/op
// BenchmarkPutUpload/count_10000_parallel_4-8       	       2	 532784228 ns/op	216667080 B/op	  241910 allocs/op
// BenchmarkPutUpload/count_10000_parallel_8-8       	       3	 494290188 ns/op	216297749 B/op	  236247 allocs/op
// BenchmarkPutUpload/count_10000_parallel_16-8      	       3	 483485315 ns/op	216060384 B/op	  231090 allocs/op
// BenchmarkPutUpload/count_10000_parallel_32-8      	       3	 434461294 ns/op	215371280 B/op	  224800 allocs/op
// BenchmarkPutUpload/count_100000_parallel_1-8      	       1	22767894338 ns/op	2331372088 B/op	 4049876 allocs/op
// BenchmarkPutUpload/count_100000_parallel_2-8      	       1	25347872677 ns/op	2344140160 B/op	 4106763 allocs/op
// BenchmarkPutUpload/count_100000_parallel_4-8      	       1	23580460174 ns/op	2338582576 B/op	 4027452 allocs/op
// BenchmarkPutUpload/count_100000_parallel_8-8      	       1	22197559193 ns/op	2321803496 B/op	 3877553 allocs/op
// BenchmarkPutUpload/count_100000_parallel_16-8     	       1	22527046476 ns/op	2327854800 B/op	 3885455 allocs/op
// BenchmarkPutUpload/count_100000_parallel_32-8     	       1	21332243613 ns/op	2299654568 B/op	 3697181 allocs/op
// PASS
func BenchmarkPutUpload(b *testing.B) {
	for _, count := range []int{
		100,
		1000,
		10000,
		100000,
	} {
		for _, maxParallelUploads := range []int{
			1,
			2,
			4,
			8,
			16,
			32,
		} {
			name := fmt.Sprintf("count %v parallel %v", count, maxParallelUploads)
			b.Run(name, func(b *testing.B) {
				for n := 0; n < b.N; n++ {
					benchmarkPutUpload(b, nil, count, maxParallelUploads)
				}
			})
		}
	}
}

// benchmarkPutUpload runs a benchmark by uploading a specific number
// of chunks with specified max parallel uploads.
func benchmarkPutUpload(b *testing.B, o *Options, count, maxParallelUploads int) {
	b.StopTimer()
	db := newTestDB(b, o)

	chunks := make([]swarm.Chunk, count)
	for i := 0; i < count; i++ {
		chunks[i] = generateTestRandomChunk()
	}
	errs := make(chan error)
	b.StartTimer()

	go func() {
		sem := make(chan struct{}, maxParallelUploads)
		for i := 0; i < count; i++ {
			sem <- struct{}{}

			go func(i int) {
				defer func() { <-sem }()

				_, err := db.Put(context.Background(), storage.ModePutUpload, chunks[i])
				errs <- err
			}(i)
		}
	}()

	for i := 0; i < count; i++ {
		err := <-errs
		if err != nil {
			b.Fatal(err)
		}
	}
}
