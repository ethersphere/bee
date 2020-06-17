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
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestDB_SubscribePull_first is a regression test for the first=false (from-1) bug
// The bug was that `first=false` was not behind an if-condition `if count > 0`. This resulted in chunks being missed, when
// the subscription is established before the chunk is actually uploaded. For example if a subscription is established with since=49,
// which means that the `SubscribePull` method should return chunk with BinID=49 via the channel, and the chunk for BinID=49 is uploaded,
// after the subscription, then it would have been skipped, where the correct behaviour is to not skip it and return it via the channel.
func TestDB_SubscribePull_first(t *testing.T) {
	db := newTestDB(t, nil)

	addrs := make(map[uint8][]swarm.Address)
	var addrsMu sync.Mutex
	var wantedChunksCount int

	// prepopulate database with some chunks
	// before the subscription
	uploadRandomChunksBin(t, db, addrs, &addrsMu, &wantedChunksCount, 100)

	// any bin should do the trick
	bin := uint8(1)

	chunksInGivenBin := uint64(len(addrs[bin]))

	errc := make(chan error)

	since := chunksInGivenBin + 1

	go func() {
		ch, _, stop := db.SubscribePull(context.TODO(), bin, since, 0)
		defer stop()

		chnk := <-ch

		if chnk.BinID != since {
			errc <- fmt.Errorf("expected chunk.BinID to be %v , but got %v", since, chnk.BinID)
		} else {
			errc <- nil
		}
	}()

	time.Sleep(100 * time.Millisecond)

	uploadRandomChunksBin(t, db, addrs, &addrsMu, &wantedChunksCount, 100)

	err := <-errc
	if err != nil {
		t.Fatal(err)
	}
}

// TestDB_SubscribePull uploads some chunks before and after
// pull syncing subscription is created and validates if
// all addresses are received in the right order
// for expected proximity order bins.
func TestDB_SubscribePull(t *testing.T) {
	db := newTestDB(t, nil)

	addrs := make(map[uint8][]swarm.Address)
	var addrsMu sync.Mutex
	var wantedChunksCount int

	// prepopulate database with some chunks
	// before the subscription
	uploadRandomChunksBin(t, db, addrs, &addrsMu, &wantedChunksCount, 10)

	// set a timeout on subscription
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// collect all errors from validating addresses, even nil ones
	// to validate the number of addresses received by the subscription
	errChan := make(chan error)

	for bin := uint8(0); bin <= swarm.MaxPO; bin++ {
		ch, _, stop := db.SubscribePull(ctx, bin, 0, 0)
		defer stop()

		// receive and validate addresses from the subscription
		go readPullSubscriptionBin(ctx, db, bin, ch, addrs, &addrsMu, errChan)
	}

	// upload some chunks just after subscribe
	uploadRandomChunksBin(t, db, addrs, &addrsMu, &wantedChunksCount, 5)

	time.Sleep(200 * time.Millisecond)

	// upload some chunks after some short time
	// to ensure that subscription will include them
	// in a dynamic environment
	uploadRandomChunksBin(t, db, addrs, &addrsMu, &wantedChunksCount, 3)

	checkErrChan(ctx, t, errChan, wantedChunksCount)
}

// TestDB_SubscribePull_multiple uploads chunks before and after
// multiple pull syncing subscriptions are created and
// validates if all addresses are received in the right order
// for expected proximity order bins.
func TestDB_SubscribePull_multiple(t *testing.T) {
	db := newTestDB(t, nil)

	addrs := make(map[uint8][]swarm.Address)
	var addrsMu sync.Mutex
	var wantedChunksCount int

	// prepopulate database with some chunks
	// before the subscription
	uploadRandomChunksBin(t, db, addrs, &addrsMu, &wantedChunksCount, 10)

	// set a timeout on subscription
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// collect all errors from validating addresses, even nil ones
	// to validate the number of addresses received by the subscription
	errChan := make(chan error)

	subsCount := 10

	// start a number of subscriptions
	// that all of them will write every address error to errChan
	for j := 0; j < subsCount; j++ {
		for bin := uint8(0); bin <= swarm.MaxPO; bin++ {
			ch, _, stop := db.SubscribePull(ctx, bin, 0, 0)
			defer stop()

			// receive and validate addresses from the subscription
			go readPullSubscriptionBin(ctx, db, bin, ch, addrs, &addrsMu, errChan)
		}
	}

	// upload some chunks just after subscribe
	uploadRandomChunksBin(t, db, addrs, &addrsMu, &wantedChunksCount, 5)

	time.Sleep(200 * time.Millisecond)

	// upload some chunks after some short time
	// to ensure that subscription will include them
	// in a dynamic environment
	uploadRandomChunksBin(t, db, addrs, &addrsMu, &wantedChunksCount, 3)

	checkErrChan(ctx, t, errChan, wantedChunksCount*subsCount)
}

// TestDB_SubscribePull_since uploads chunks before and after
// pull syncing subscriptions are created with a since argument
// and validates if all expected addresses are received in the
// right order for expected proximity order bins.
func TestDB_SubscribePull_since(t *testing.T) {
	db := newTestDB(t, nil)

	addrs := make(map[uint8][]swarm.Address)
	var addrsMu sync.Mutex
	var wantedChunksCount int

	binIDCounter := make(map[uint8]uint64)
	var binIDCounterMu sync.RWMutex

	uploadRandomChunks := func(count int, wanted bool) (first map[uint8]uint64) {
		addrsMu.Lock()
		defer addrsMu.Unlock()

		first = make(map[uint8]uint64)
		for i := 0; i < count; i++ {
			ch := generateTestRandomChunk()

			_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
			if err != nil {
				t.Fatal(err)
			}

			bin := db.po(ch.Address())

			binIDCounterMu.RLock()
			binIDCounter[bin]++
			binIDCounterMu.RUnlock()

			if wanted {
				if _, ok := addrs[bin]; !ok {
					addrs[bin] = make([]swarm.Address, 0)
				}
				addrs[bin] = append(addrs[bin], ch.Address())
				wantedChunksCount++

				if _, ok := first[bin]; !ok {
					first[bin] = binIDCounter[bin]
				}
			}
		}
		return first
	}

	// prepopulate database with some chunks
	// before the subscription
	uploadRandomChunks(30, false)

	first := uploadRandomChunks(25, true)

	// set a timeout on subscription
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// collect all errors from validating addresses, even nil ones
	// to validate the number of addresses received by the subscription
	errChan := make(chan error)

	for bin := uint8(0); bin <= swarm.MaxPO; bin++ {
		since, ok := first[bin]
		if !ok {
			continue
		}
		ch, _, stop := db.SubscribePull(ctx, bin, since, 0)
		defer stop()

		// receive and validate addresses from the subscription
		go readPullSubscriptionBin(ctx, db, bin, ch, addrs, &addrsMu, errChan)

	}

	checkErrChan(ctx, t, errChan, wantedChunksCount)
}

// TestDB_SubscribePull_until uploads chunks before and after
// pull syncing subscriptions are created with an until argument
// and validates if all expected addresses are received in the
// right order for expected proximity order bins.
func TestDB_SubscribePull_until(t *testing.T) {
	db := newTestDB(t, nil)

	addrs := make(map[uint8][]swarm.Address)
	var addrsMu sync.Mutex
	var wantedChunksCount int

	binIDCounter := make(map[uint8]uint64)
	var binIDCounterMu sync.RWMutex

	uploadRandomChunks := func(count int, wanted bool) (last map[uint8]uint64) {
		addrsMu.Lock()
		defer addrsMu.Unlock()

		last = make(map[uint8]uint64)
		for i := 0; i < count; i++ {
			ch := generateTestRandomChunk()

			_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
			if err != nil {
				t.Fatal(err)
			}

			bin := db.po(ch.Address())

			if _, ok := addrs[bin]; !ok {
				addrs[bin] = make([]swarm.Address, 0)
			}
			if wanted {
				addrs[bin] = append(addrs[bin], ch.Address())
				wantedChunksCount++
			}

			binIDCounterMu.RLock()
			binIDCounter[bin]++
			binIDCounterMu.RUnlock()

			last[bin] = binIDCounter[bin]
		}
		return last
	}

	// prepopulate database with some chunks
	// before the subscription
	last := uploadRandomChunks(30, true)

	uploadRandomChunks(25, false)

	// set a timeout on subscription
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// collect all errors from validating addresses, even nil ones
	// to validate the number of addresses received by the subscription
	errChan := make(chan error)

	for bin := uint8(0); bin <= swarm.MaxPO; bin++ {
		until, ok := last[bin]
		if !ok {
			continue
		}
		ch, _, stop := db.SubscribePull(ctx, bin, 0, until)
		defer stop()

		// receive and validate addresses from the subscription
		go readPullSubscriptionBin(ctx, db, bin, ch, addrs, &addrsMu, errChan)
	}

	// upload some chunks just after subscribe
	uploadRandomChunks(15, false)

	checkErrChan(ctx, t, errChan, wantedChunksCount)
}

// TestDB_SubscribePull_sinceAndUntil uploads chunks before and
// after pull syncing subscriptions are created with since
// and until arguments, and validates if all expected addresses
// are received in the right order for expected proximity order bins.
func TestDB_SubscribePull_sinceAndUntil(t *testing.T) {
	db := newTestDB(t, nil)

	addrs := make(map[uint8][]swarm.Address)
	var addrsMu sync.Mutex
	var wantedChunksCount int

	binIDCounter := make(map[uint8]uint64)
	var binIDCounterMu sync.RWMutex

	uploadRandomChunks := func(count int, wanted bool) (last map[uint8]uint64) {
		addrsMu.Lock()
		defer addrsMu.Unlock()

		last = make(map[uint8]uint64)
		for i := 0; i < count; i++ {
			ch := generateTestRandomChunk()

			_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
			if err != nil {
				t.Fatal(err)
			}

			bin := db.po(ch.Address())

			if _, ok := addrs[bin]; !ok {
				addrs[bin] = make([]swarm.Address, 0)
			}
			if wanted {
				addrs[bin] = append(addrs[bin], ch.Address())
				wantedChunksCount++
			}

			binIDCounterMu.RLock()
			binIDCounter[bin]++
			binIDCounterMu.RUnlock()

			last[bin] = binIDCounter[bin]
		}
		return last
	}

	// all chunks from upload1 are not expected
	// as upload1 chunk is used as since for subscriptions
	upload1 := uploadRandomChunks(100, false)

	// all chunks from upload2 are expected
	// as upload2 chunk is used as until for subscriptions
	upload2 := uploadRandomChunks(100, true)

	// upload some chunks before subscribe but after
	// wanted chunks
	uploadRandomChunks(8, false)

	// set a timeout on subscription
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// collect all errors from validating addresses, even nil ones
	// to validate the number of addresses received by the subscription
	errChan := make(chan error)

	for bin := uint8(0); bin <= swarm.MaxPO; bin++ {
		since, ok := upload1[bin]
		if ok {
			// start from the next uploaded chunk
			since++
		}
		until, ok := upload2[bin]
		if !ok {
			// no chunks un this bin uploaded in the upload2
			// skip this bin from testing
			continue
		}
		ch, _, stop := db.SubscribePull(ctx, bin, since, until)
		defer stop()

		// receive and validate addresses from the subscription
		go readPullSubscriptionBin(ctx, db, bin, ch, addrs, &addrsMu, errChan)
	}

	// upload some chunks just after subscribe
	uploadRandomChunks(15, false)

	checkErrChan(ctx, t, errChan, wantedChunksCount)
}

// TestDB_SubscribePull_rangeOnRemovedChunks performs a test:
// - uploads a number of chunks
// - removes first half of chunks for every bin
// - subscribes to a range that is within removed chunks,
//   but before the chunks that are left
// - validates that no chunks are received on subscription channel
func TestDB_SubscribePull_rangeOnRemovedChunks(t *testing.T) {
	db := newTestDB(t, nil)

	// keeps track of available chunks in the database
	// per bin with their bin ids
	chunks := make(map[uint8][]storage.Descriptor)

	// keeps track of latest bin id for every bin
	binIDCounter := make(map[uint8]uint64)

	// upload chunks to populate bins from start
	// bin ids start from 1
	const chunkCount = 1000
	for i := 0; i < chunkCount; i++ {
		ch := generateTestRandomChunk()

		_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}

		bin := db.po(ch.Address())

		binIDCounter[bin]++
		binID := binIDCounter[bin]

		if _, ok := chunks[bin]; !ok {
			chunks[bin] = make([]storage.Descriptor, 0)
		}
		chunks[bin] = append(chunks[bin], storage.Descriptor{
			Address: ch.Address(),
			BinID:   binID,
		})
	}

	// remove first half of the chunks in every bin
	for bin := range chunks {
		count := len(chunks[bin])
		for i := 0; i < count/2; i++ {
			d := chunks[bin][0]
			if err := db.Set(context.Background(), storage.ModeSetRemove, d.Address); err != nil {
				t.Fatal(err)
			}
			chunks[bin] = chunks[bin][1:]
		}
	}

	// set a timeout on subscription
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// signals that there were valid bins for this check to ensure test validity
	var checkedBins int
	// subscribe to every bin and validate returned values
	for bin := uint8(0); bin <= swarm.MaxPO; bin++ {
		// do not subscribe to bins that do not have chunks
		if len(chunks[bin]) == 0 {
			continue
		}
		// subscribe from start of the bin index
		var since uint64
		// subscribe until the first available chunk in bin,
		// but not for it
		until := chunks[bin][0].BinID - 1
		if until == 0 {
			// ignore this bin if it has only one chunk left
			continue
		}
		ch, _, stop := db.SubscribePull(ctx, bin, since, until)
		defer stop()

		// the returned channel should be closed
		// because no chunks should be provided
		select {
		case d, ok := <-ch:
			if !ok {
				// this is expected for successful case
				break
			}
			if d.BinID > until {
				t.Errorf("got %v for bin %v, subscribed until bin id %v", d.BinID, bin, until)
			}
		case <-ctx.Done():
			t.Error(ctx.Err())
		}

		// mark that the check is performed
		checkedBins++
	}
	// check that test performed at least one validation
	if checkedBins == 0 {
		t.Fatal("test did not perform any checks")
	}
}

// uploadRandomChunksBin uploads random chunks to database and adds them to
// the map of addresses ber bin.
func uploadRandomChunksBin(t *testing.T, db *DB, addrs map[uint8][]swarm.Address, addrsMu *sync.Mutex, wantedChunksCount *int, count int) {
	addrsMu.Lock()
	defer addrsMu.Unlock()

	for i := 0; i < count; i++ {
		ch := generateTestRandomChunk()

		_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}

		bin := db.po(ch.Address())
		if _, ok := addrs[bin]; !ok {
			addrs[bin] = make([]swarm.Address, 0)
		}
		addrs[bin] = append(addrs[bin], ch.Address())

		*wantedChunksCount++
	}
}

// readPullSubscriptionBin is a helper function that reads all storage.Descriptors from a channel and
// sends error to errChan, even if it is nil, to count the number of storage.Descriptors
// returned by the channel.
func readPullSubscriptionBin(ctx context.Context, db *DB, bin uint8, ch <-chan storage.Descriptor, addrs map[uint8][]swarm.Address, addrsMu *sync.Mutex, errChan chan error) {
	var i int // address index
	for {
		select {
		case got, ok := <-ch:
			if !ok {
				return
			}
			var err error
			addrsMu.Lock()
			if i+1 > len(addrs[bin]) {
				err = fmt.Errorf("got more chunk addresses %v, then expected %v, for bin %v", i+1, len(addrs[bin]), bin)
			} else {
				addr := addrs[bin][i]
				if !got.Address.Equal(addr) {
					err = fmt.Errorf("got chunk bin id %v in bin %v %v, want %v", i, bin, got.Address, addr)
				} else {
					var want shed.Item
					want, err = db.retrievalDataIndex.Get(shed.Item{
						Address: addr.Bytes(),
					})
					if err != nil {
						err = fmt.Errorf("got chunk (bin id %v in bin %v) from retrieval index %s: %v", i, bin, addrs[bin][i], err)
					} else if got.BinID != want.BinID {
						err = fmt.Errorf("got chunk bin id %v in bin %v %v, want %v", i, bin, got, want)
					}
				}
			}
			addrsMu.Unlock()
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
}

// checkErrChan expects the number of wantedChunksCount errors from errChan
// and calls t.Error for the ones that are not nil.
func checkErrChan(ctx context.Context, t *testing.T, errChan chan error, wantedChunksCount int) {
	t.Helper()

	for i := 0; i < wantedChunksCount; i++ {
		select {
		case err := <-errChan:
			if err != nil {
				t.Error(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}

// TestDB_LastPullSubscriptionBinID validates that LastPullSubscriptionBinID
// is returning the last chunk descriptor for proximity order bins by
// doing a few rounds of chunk uploads.
func TestDB_LastPullSubscriptionBinID(t *testing.T) {
	db := newTestDB(t, nil)

	addrs := make(map[uint8][]swarm.Address)

	binIDCounter := make(map[uint8]uint64)
	var binIDCounterMu sync.RWMutex

	last := make(map[uint8]uint64)

	// do a few rounds of uploads and check if
	// last pull subscription chunk is correct
	for _, count := range []int{1, 3, 10, 11, 100, 120} {

		// upload
		for i := 0; i < count; i++ {
			ch := generateTestRandomChunk()

			_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
			if err != nil {
				t.Fatal(err)
			}

			bin := db.po(ch.Address())

			if _, ok := addrs[bin]; !ok {
				addrs[bin] = make([]swarm.Address, 0)
			}
			addrs[bin] = append(addrs[bin], ch.Address())

			binIDCounterMu.RLock()
			binIDCounter[bin]++
			binIDCounterMu.RUnlock()

			last[bin] = binIDCounter[bin]
		}

		// check
		for bin := uint8(0); bin <= swarm.MaxPO; bin++ {
			want, ok := last[bin]
			got, err := db.LastPullSubscriptionBinID(bin)
			if ok {
				if err != nil {
					t.Errorf("got unexpected error value %v", err)
				}
			}
			if got != want {
				t.Errorf("got last bin id %v, want %v", got, want)
			}
		}
	}
}

// TestAddressInBin validates that function addressInBin
// returns a valid address for every proximity order bin.
func TestAddressInBin(t *testing.T) {
	db := newTestDB(t, nil)

	for po := uint8(0); po < swarm.MaxPO; po++ {
		addr := db.addressInBin(po)

		got := db.po(addr)

		if got != po {
			t.Errorf("got po %v, want %v", got, po)
		}
	}
}
