// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullstorage_test

import (
	"context"
	"crypto/rand"
	"errors"
	"io/ioutil"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/ethersphere/bee/pkg/localstore"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pullsync/pullstorage"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	stesting "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	addrs = []swarm.Address{
		swarm.MustParseHexAddress("0001"),
		swarm.MustParseHexAddress("0002"),
		swarm.MustParseHexAddress("0003"),
		swarm.MustParseHexAddress("0004"),
		swarm.MustParseHexAddress("0005"),
		swarm.MustParseHexAddress("0006"),
	}
	limit = 5
)

func someAddrs(i ...int) (r []swarm.Address) {
	for _, v := range i {
		r = append(r, addrs[v])
	}
	return r
}

func someDescriptors(i ...int) (d []storage.Descriptor) {
	for _, v := range i {
		d = append(d, storage.Descriptor{Address: addrs[v], BinID: uint64(v + 1)})
	}
	return d
}

// TestIntervalChunks tests that the IntervalChunks method always returns
// an upper bound of N chunks for a certain range, and the topmost equals:
// - to the To argument of the function (in case there are no chunks in the interval)
// - to the To argument of the function (in case the number of chunks in interval <= N)
// - to BinID of the last chunk in the returned collection in case number of chunks in interval > N
func TestIntervalChunks(t *testing.T) {

	// we need to check four cases of the subscribe pull iterator:
	// - no chunks in interval
	// - less chunks reported than what is in the interval (but interval still intact, probably old chunks GCd)
	// - as much chunks as size of interval
	// - more chunks than what's in interval (return lower topmost value)
	// - less chunks in interval, but since we're at the top of the interval, block and wait for new chunks

	for _, tc := range []struct {
		desc     string
		from, to uint64 // request from, to

		mockAddrs []int  // which addresses should the mock return
		addrs     []byte // the expected returned chunk address byte slice
		topmost   uint64 // expected topmost
	}{
		{desc: "no chunks in interval", from: 0, to: 5, topmost: 5},
		{desc: "interval full", from: 0, to: 5, mockAddrs: []int{0, 1, 2, 3, 4}, topmost: 5},
		{desc: "some in the middle", from: 0, to: 5, mockAddrs: []int{1, 3}, topmost: 5},
		{desc: "at the edges", from: 0, to: 5, mockAddrs: []int{0, 4}, topmost: 5},
		{desc: "at the edges and the middle", from: 0, to: 5, mockAddrs: []int{0, 2, 4}, topmost: 5},
		{desc: "more than interval", from: 0, to: 5, mockAddrs: []int{0, 1, 2, 3, 4, 5}, topmost: 5},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			b := someAddrs(tc.mockAddrs...)
			desc := someDescriptors(tc.mockAddrs...)
			ps, _ := newPullStorage(t, mock.WithSubscribePullChunks(desc...))
			ctx, cancel := context.WithCancel(context.Background())

			addresses, topmost, err := ps.IntervalChunks(ctx, 0, tc.from, tc.to, limit)
			if err != nil {
				t.Fatal(err)
			}

			cancel()

			checkAinB(t, addresses, b)

			if topmost != tc.topmost {
				t.Fatalf("expected topmost %d but got %d", tc.topmost, topmost)
			}
		})
	}
}

// Get some descriptor from the chunk channel, then block for a while
// then add more chunks to the subscribe pull iterator and make sure the loop
// exits correctly.
func TestIntervalChunks_GetChunksLater(t *testing.T) {
	desc := someDescriptors(0, 2)
	ps, db := newPullStorage(t, mock.WithSubscribePullChunks(desc...), mock.WithPartialInterval(true))

	go func() {
		<-time.After(200 * time.Millisecond)
		// add chunks to subscribe pull on the storage mock
		db.MorePull(someDescriptors(1, 3, 4)...)
	}()

	addrs, topmost, err := ps.IntervalChunks(context.Background(), 0, 0, 5, limit)
	if err != nil {
		t.Fatal(err)
	}

	if l := len(addrs); l != 5 {
		t.Fatalf("want %d addrs but got %d", 5, l)
	}

	// highest chunk we sent had BinID 5
	exp := uint64(5)
	if topmost != exp {
		t.Fatalf("expected topmost %d but got %d", exp, topmost)
	}
}

func TestIntervalChunks_Blocking(t *testing.T) {
	desc := someDescriptors(0, 2)
	ps, _ := newPullStorage(t, mock.WithSubscribePullChunks(desc...), mock.WithPartialInterval(true))
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		<-time.After(100 * time.Millisecond)
		cancel()
	}()

	_, _, err := ps.IntervalChunks(ctx, 0, 0, 5, limit)
	if err == nil {
		t.Fatal("expected error but got none")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestIntervalChunks_DbShutdown(t *testing.T) {
	ps, db := newPullStorage(t, mock.WithPartialInterval(true))

	go func() {
		<-time.After(100 * time.Millisecond)
		db.Close()
	}()

	_, _, err := ps.IntervalChunks(context.Background(), 0, 0, 5, limit)
	if err == nil {
		t.Fatal("expected error but got none")
	}

	if !errors.Is(err, pullstorage.ErrDbClosed) {
		t.Fatal(err)
	}
}

// TestIntervalChunks_Localstore is an integration test with a real
// localstore instance.
func TestIntervalChunks_Localstore(t *testing.T) {
	fill := func(f, t int) (ints []int) {
		for i := f; i <= t; i++ {
			ints = append(ints, i)
		}
		return ints
	}
	for _, tc := range []struct {
		name   string
		chunks int
		f, t   uint64
		limit  int
		expect int    // chunks
		top    uint64 // topmost
		addrs  []int  // indexes of the generated chunk slice
	}{
		{
			name:   "0-1, expect 1 chunk", // intervals always >0
			chunks: 50,
			f:      0, t: 1,
			limit:  50,
			expect: 1, top: 1, addrs: fill(1, 1),
		},
		{
			name:   "1-1, expect 1 chunk",
			chunks: 50,
			f:      1, t: 1,
			limit:  50,
			expect: 1, top: 1, addrs: fill(1, 1),
		},
		{
			name:   "2-2, expect 1 chunk",
			chunks: 50,
			f:      2, t: 2,
			limit:  50,
			expect: 1, top: 2, addrs: fill(2, 2),
		},
		{
			name:   "0-10, expect 10 chunks", // intervals always >0
			chunks: 50,
			f:      0, t: 10,
			limit:  50,
			expect: 10, top: 10, addrs: fill(1, 10),
		},
		{
			name:   "1-10, expect 10 chunks",
			chunks: 50,
			f:      0, t: 10,
			limit:  50,
			expect: 10, top: 10, addrs: fill(1, 10),
		},

		{
			name:   "0-50, expect 50 chunks", // intervals always >0
			chunks: 50,
			f:      0, t: 50,
			limit:  50,
			expect: 50, top: 50, addrs: fill(1, 50),
		},
		{
			name:   "1-50, expect 50 chunks",
			chunks: 50,
			f:      1, t: 50,
			limit:  50,
			expect: 50, top: 50, addrs: fill(1, 50),
		},
		{
			name:   "0-60, expect 50 chunks", // hit the limit
			chunks: 50,
			f:      0, t: 60,
			limit:  50,
			expect: 50, top: 50, addrs: fill(1, 50),
		},
		{
			name:   "1-60, expect 50 chunks", // hit the limit
			chunks: 50,
			f:      0, t: 60,
			limit:  50,
			expect: 50, top: 50, addrs: fill(1, 50),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {

			base, db := newTestDB(t, nil)
			ps := pullstorage.New(db)

			var chunks []swarm.Chunk

			for i := 1; i <= tc.chunks; {
				c := stesting.GenerateTestRandomChunk()
				po := swarm.Proximity(c.Address().Bytes(), base)
				if po == 1 {
					chunks = append(chunks, c)
					i++
				}
			}

			ctx := context.Background()
			_, err := db.Put(ctx, storage.ModePutUpload, chunks...)
			if err != nil {
				t.Fatal(err)
			}

			//always bin 1
			chs, topmost, err := ps.IntervalChunks(ctx, 1, tc.f, tc.t, tc.limit)
			if err != nil {
				t.Fatal(err)
			}

			checkAddrs := make([]swarm.Address, len(tc.addrs))
			for i, v := range tc.addrs {
				checkAddrs[i] = chunks[v-1].Address()
			}

			for i, c := range chs {
				if !c.Equal(checkAddrs[i]) {
					t.Fatalf("chunk %d address mismatch", i)
				}
			}

			if topmost != tc.top {
				t.Fatalf("topmost mismatch, got %d want %d", topmost, tc.top)
			}

			if l := len(chs); l != tc.expect {
				t.Fatalf("expected %d chunks but got %d", tc.expect, l)
			}

		})
	}
}

// TestIntervalChunks_IteratorShare tests that two goroutines
// with the same subscription call the SubscribePull only once
// and that results are shared between both of them.
func TestIntervalChunks_IteratorShare(t *testing.T) {
	desc := someDescriptors(0, 2)
	ps, db := newPullStorage(t, mock.WithSubscribePullChunks(desc...), mock.WithPartialInterval(true))

	go func() {
		// delay is needed in order to have the iterator
		// linger for a bit longer for more chunks.
		<-time.After(200 * time.Millisecond)
		// add chunks to subscribe pull on the storage mock
		db.MorePull(someDescriptors(1, 3, 4)...)
	}()

	type result struct {
		addrs []swarm.Address
		top   uint64
	}
	sched := make(chan struct{})
	c := make(chan result)

	go func() {
		close(sched)
		addrs, topmost, err := ps.IntervalChunks(context.Background(), 0, 0, 5, limit)
		if err != nil {
			t.Errorf("internal goroutine: %v", err)
		}
		c <- result{addrs, topmost}

	}()
	<-sched // wait for goroutine to get scheduled

	addrs, topmost, err := ps.IntervalChunks(context.Background(), 0, 0, 5, limit)
	if err != nil {
		t.Fatal(err)
	}

	res := <-c

	if l := len(addrs); l != 5 {
		t.Fatalf("want %d addrs but got %d", 5, l)
	}

	// highest chunk we sent had BinID 5
	exp := uint64(5)
	if topmost != exp {
		t.Fatalf("expected topmost %d but got %d", exp, topmost)
	}
	if c := db.SubscribePullCalls(); c != 1 {
		t.Fatalf("wanted 1 subscribe pull calls, got %d", c)
	}

	// check that results point to same array
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&res.addrs))
	sh2 := (*reflect.SliceHeader)(unsafe.Pointer(&addrs))

	if sh.Data != sh2.Data {
		t.Fatalf("results not shared between goroutines. ptr1 %d ptr2 %d", sh.Data, sh2.Data)
	}
}

func newPullStorage(t *testing.T, o ...mock.Option) (pullstorage.Storer, *mock.MockStorer) {
	db := mock.NewStorer(o...)
	ps := pullstorage.New(db)

	return ps, db
}

func newTestDB(t testing.TB, o *localstore.Options) (baseKey []byte, db *localstore.DB) {
	t.Helper()

	baseKey = make([]byte, 32)
	if _, err := rand.Read(baseKey); err != nil {
		t.Fatal(err)
	}

	logger := logging.New(ioutil.Discard, 0)
	db, err := localstore.New("", baseKey, o, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		err := db.Close()
		if err != nil {
			t.Error(err)
		}
	})
	return baseKey, db
}

// check that every a exists in b
func checkAinB(t *testing.T, a, b []swarm.Address) {
	t.Helper()
	for _, v := range a {
		if !isIn(v, b) {
			t.Fatalf("address %s not found in slice %s", v, b)
		}
	}
}

func isIn(a swarm.Address, b []swarm.Address) bool {
	for _, v := range b {
		if a.Equal(v) {
			return true
		}
	}
	return false
}
