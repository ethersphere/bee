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
	"strconv"
	"testing"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// BenchmarkRetrievalIndexes uploads a number of chunks in order to measure
// total time of updating their retrieval indexes by setting them
// to synced state and requesting them.
//
// This benchmark takes significant amount of time.
//
// Measurements on MacBook Pro (Retina, 15-inch, Mid 2014) show
// that two separated indexes perform better.
//
// # go test -benchmem -run=none github.com/ethersphere/swarm/storage/localstore -bench BenchmarkRetrievalIndexes -v
// goos: darwin
// goarch: amd64
// pkg: github.com/ethersphere/swarm/storage/localstore
// BenchmarkRetrievalIndexes/1000-8         	      20       75556686 ns/op      19033493 B/op       84500 allocs/op
// BenchmarkRetrievalIndexes/10000-8        	       1     1079084922 ns/op     382792064 B/op     1429644 allocs/op
// BenchmarkRetrievalIndexes/100000-8       	       1    16891305737 ns/op    2629165304 B/op    12465019 allocs/op
// PASS
func BenchmarkRetrievalIndexes(b *testing.B) {
	for _, count := range []int{
		1000,
		10000,
		100000,
	} {
		b.Run(strconv.Itoa(count)+"-split", func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				benchmarkRetrievalIndexes(b, nil, count)
			}
		})
	}
}

// benchmarkRetrievalIndexes is used in BenchmarkRetrievalIndexes
// to do benchmarks with a specific number of chunks and different
// database options.
func benchmarkRetrievalIndexes(b *testing.B, o *Options, count int) {
	b.Helper()

	b.StopTimer()
	db := newTestDB(b, o)
	addrs := make([]swarm.Address, count)
	for i := 0; i < count; i++ {
		ch := generateTestRandomChunk()
		_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
		if err != nil {
			b.Fatal(err)
		}
		addrs[i] = ch.Address()
	}
	// set update gc test hook to signal when
	// update gc goroutine is done by sending to
	// testHookUpdateGCChan channel, which is
	// used to wait for gc index updates to be
	// included in the benchmark time
	testHookUpdateGCChan := make(chan struct{})
	defer setTestHookUpdateGC(func() {
		testHookUpdateGCChan <- struct{}{}
	})()
	b.StartTimer()

	for i := 0; i < count; i++ {
		err := db.Set(context.Background(), storage.ModeSetSync, addrs[i])
		if err != nil {
			b.Fatal(err)
		}

		_, err = db.Get(context.Background(), storage.ModeGetRequest, addrs[i])
		if err != nil {
			b.Fatal(err)
		}
		// wait for update gc goroutine to be done
		<-testHookUpdateGCChan
	}
}

// BenchmarkUpload compares uploading speed for different
// retrieval indexes and various number of chunks.
//
// Measurements on MacBook Pro (Retina, 15-inch, Mid 2014).
//
// go test -benchmem -run=none github.com/ethersphere/swarm/storage/localstore -bench BenchmarkUpload -v
// goos: darwin
// goarch: amd64
// pkg: github.com/ethersphere/swarm/storage/localstore
// BenchmarkUpload/1000-8         	      20       59437463 ns/op     25205193 B/op    23208 allocs/op
// BenchmarkUpload/10000-8        	       2      580646362 ns/op    216532932 B/op	  248090 allocs/op
// BenchmarkUpload/100000-8       	       1    22373390892 ns/op   2323055312 B/op	 3995903 allocs/op
// PASS
func BenchmarkUpload(b *testing.B) {
	for _, count := range []int{
		1000,
		10000,
		100000,
	} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				benchmarkUpload(b, nil, count)
			}
		})
	}
}

// benchmarkUpload is used in BenchmarkUpload
// to do benchmarks with a specific number of chunks and different
// database options.
func benchmarkUpload(b *testing.B, o *Options, count int) {
	b.Helper()

	b.StopTimer()
	db := newTestDB(b, o)
	chunks := make([]swarm.Chunk, count)
	for i := 0; i < count; i++ {
		chunk := generateTestRandomChunk()
		chunks[i] = chunk
	}
	b.StartTimer()

	for i := 0; i < count; i++ {
		_, err := db.Put(context.Background(), storage.ModePutUpload, chunks[i])
		if err != nil {
			b.Fatal(err)
		}
	}
}
