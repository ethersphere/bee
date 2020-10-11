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
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestDB_SubscribePush uploads some chunks before and after
// push syncing subscription is created and validates if
// all addresses are received in the right order.
func TestDB_SubscribePush(t *testing.T) {
	db := newTestDB(t, nil)

	chunks := make([]swarm.Chunk, 0)
	var chunksMu sync.Mutex

	chunkProcessedTimes := make([]int, 0)

	uploadRandomChunks := func(count int) {
		chunksMu.Lock()
		defer chunksMu.Unlock()

		for i := 0; i < count; i++ {
			ch := generateTestRandomChunk()

			_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
			if err != nil {
				t.Fatal(err)
			}

			chunks = append(chunks, ch)

			chunkProcessedTimes = append(chunkProcessedTimes, 0)
		}
	}

	// REQUIRED_LOCK - chunksMu
	findChunkIndex := func(chunk swarm.Chunk) int {
		for i, c := range chunks {
			if chunk.Address().Equal(c.Address()) {
				return i
			}
		}

		return -1
	}

	// prepopulate database with some chunks
	// before the subscription
	uploadRandomChunks(10)

	// set a timeout on subscription
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// collect all errors from validating addresses, even nil ones
	// to validate the number of addresses received by the subscription
	errChan := make(chan error)

	ch, stop := db.SubscribePush(ctx)
	defer stop()

	var lastStartIndex int = -1

	// receive and validate addresses from the subscription
	go func() {
		var err error
		var i int // address index
		for {
			select {
			case got, ok := <-ch:
				if !ok {
					return
				}
				chunksMu.Lock()
				if i > lastStartIndex {
					// no way to know which chunk will come first here
					gotIndex := findChunkIndex(got)
					if gotIndex <= lastStartIndex {
						err = fmt.Errorf("got index %v, expected index above %v", gotIndex, lastStartIndex)
					}
					lastStartIndex = gotIndex
					i = 0
				}
				cIndex := lastStartIndex - i
				want := chunks[cIndex]
				chunkProcessedTimes[cIndex]++
				chunksMu.Unlock()
				if !bytes.Equal(got.Data(), want.Data()) {
					err = fmt.Errorf("got chunk %v data %x, want %x", i, got.Data(), want.Data())
				}
				if !got.Address().Equal(want.Address()) {
					err = fmt.Errorf("got chunk %v address %s, want %s", i, got.Address(), want.Address())
				}
				i++
				// send one and only one error per received address
				select {
				case errChan <- err:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// upload some chunks just after subscribe
	uploadRandomChunks(5)

	time.Sleep(200 * time.Millisecond)

	// upload some chunks after some short time
	// to ensure that subscription will include them
	// in a dynamic environment
	uploadRandomChunks(3)

	checkErrChan(ctx, t, errChan, len(chunks))

	chunksMu.Lock()
	if lastStartIndex != len(chunks)-1 {
		t.Fatalf("got %d chunks, expected %d", lastStartIndex, len(chunks))
	}

	for i, pc := range chunkProcessedTimes {
		if pc != 1 {
			t.Fatalf("chunk on address %s processed %d times, should be only once", chunks[i].Address(), pc)
		}
	}
	chunksMu.Unlock()
}

// TestDB_SubscribePush_multiple uploads chunks before and after
// multiple push syncing subscriptions are created and
// validates if all addresses are received in the right order.
func TestDB_SubscribePush_multiple(t *testing.T) {
	db := newTestDB(t, nil)

	addrs := make([]swarm.Address, 0)
	var addrsMu sync.Mutex

	uploadRandomChunks := func(count int) {
		addrsMu.Lock()
		defer addrsMu.Unlock()

		for i := 0; i < count; i++ {
			ch := generateTestRandomChunk()

			_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
			if err != nil {
				t.Fatal(err)
			}

			addrs = append(addrs, ch.Address())
		}
	}

	// REQUIRED_LOCK - addrsMu
	findAddressIndex := func(address swarm.Address) int {
		for i, a := range addrs {
			if a.Equal(address) {
				return i
			}
		}

		return -1
	}

	// prepopulate database with some chunks
	// before the subscription
	uploadRandomChunks(10)

	// set a timeout on subscription
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// collect all errors from validating addresses, even nil ones
	// to validate the number of addresses received by the subscription
	errChan := make(chan error)

	subsCount := 10

	lastStartIndexSlice := make([]int, subsCount)

	// start a number of subscriptions
	// that all of them will write every addresses error to errChan
	for j := 0; j < subsCount; j++ {
		ch, stop := db.SubscribePush(ctx)
		defer stop()

		// receive and validate addresses from the subscription
		go func(j int) {
			var err error
			var i int // address index
			lastStartIndexSlice[j] = -1
			for {
				select {
				case got, ok := <-ch:
					if !ok {
						return
					}
					addrsMu.Lock()
					if i > lastStartIndexSlice[j] {
						// no way to know which chunk will come first here
						gotIndex := findAddressIndex(got.Address())
						if gotIndex <= lastStartIndexSlice[j] {
							err = fmt.Errorf("got index %v, expected index above %v", gotIndex, lastStartIndexSlice[j])
						}
						lastStartIndexSlice[j] = gotIndex
						i = 0
					}
					aIndex := lastStartIndexSlice[j] - i
					want := addrs[aIndex]
					addrsMu.Unlock()
					if !got.Address().Equal(want) {
						err = fmt.Errorf("got chunk %v address on subscription %v %s, want %s", i, j, got, want)
					}
					i++
					// send one and only one error per received address
					select {
					case errChan <- err:
					case <-ctx.Done():
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}(j)
	}

	// upload some chunks just after subscribe
	uploadRandomChunks(5)

	time.Sleep(200 * time.Millisecond)

	// upload some chunks after some short time
	// to ensure that subscription will include them
	// in a dynamic environment
	uploadRandomChunks(3)

	// number of addresses received by all subscriptions
	wantedChunksCount := len(addrs) * subsCount

	checkErrChan(ctx, t, errChan, wantedChunksCount)

	for j := 0; j < subsCount; j++ {
		addrsMu.Lock()
		if lastStartIndexSlice[j] != len(addrs)-1 {
			t.Fatalf("got %d chunks, expected %d", lastStartIndexSlice[j], len(addrs))
		}
		addrsMu.Unlock()
	}
}
