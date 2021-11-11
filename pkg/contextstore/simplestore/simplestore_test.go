package simplestore_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/contextstore/simplestore"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestSimpleStore(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ch := testingc.GenerateTestRandomChunk()
	if _, err := store.Put(ctx, ch); err != nil {
		t.Fatal(err)
	}

	ch2, err := store.Get(ctx, ch.Address())
	if err != nil {
		t.Fatal(err)
	}
	if !ch2.Equal(ch) {
		t.Fatal("chunks not equal")
	}
	exists, err := store.Has(ctx, ch.Address())
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected chunk to exist")
	}

	testCount(t, store, 1)

	cch := testingc.GenerateTestRandomChunk()
	if _, err = store.Put(ctx, cch); err != nil {
		t.Fatal(err)
	}
	testCount(t, store, 2)

	err = store.Delete(ctx, ch.Address())
	if err != nil {
		t.Fatal(err)
	}
	exists, err = store.Has(ctx, ch.Address())
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("expected chunk to be deleted")
	}
	testCount(t, store, 1)

	err = store.Delete(ctx, cch.Address())
	if err != nil {
		t.Fatal(err)
	}
	testCount(t, store, 0)
}

func TestSimpleStore_Iterate(t *testing.T) {
	var (
		store = newTestStore(t)
		err   error
		ctx   = context.Background()
		chs   = make(map[string]swarm.Chunk)
	)

	for i := 0; i < 100; i++ {
		ch := testingc.GenerateTestRandomChunk()
		if _, err = store.Put(ctx, ch); err != nil {
			t.Fatal(err)
		}
		chs[ch.Address().ByteString()] = ch
	}

	iterFn := func(ch swarm.Chunk) (bool, error) {
		if !ch.Equal(chs[ch.Address().ByteString()]) {
			return true, errors.New("chunks not equal")
		}
		delete(chs, ch.Address().ByteString())
		return false, nil
	}

	store.Iterate(iterFn)
	if l := len(chs); l != 0 {
		t.Fatalf("expected %d chunks got %d", 0, l)
	}
}

func TestSimpleStore_Iterate_stop(t *testing.T) {
	var (
		store = newTestStore(t)
		err   error
		ctx   = context.Background()
		chs   = make(map[string]swarm.Chunk)
	)

	for i := 0; i < 2; i++ {
		ch := testingc.GenerateTestRandomChunk()
		if _, err = store.Put(ctx, ch); err != nil {
			t.Fatal(err)
		}
		chs[ch.Address().ByteString()] = ch
	}

	iterated := 0
	err = store.Iterate(func(ch swarm.Chunk) (bool, error) {
		if !ch.Equal(chs[ch.Address().ByteString()]) {
			return true, errors.New("chunks not equal")
		}
		iterated++
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if iterated != 1 {
		t.Fatalf("expected %d iterations got %d", 1, iterated)
	}

	err = store.Iterate(func(ch swarm.Chunk) (bool, error) {
		if !ch.Equal(chs[ch.Address().ByteString()]) {
			return true, errors.New("chunks not equal")
		}
		return false, errors.New("some error")
	})
	if err == nil {
		t.Fatal("expected error but got none")
	}
}

func TestSimpleStore_parallel_r_w(t *testing.T) {
	var (
		store = newTestStore(t)
		chs   = make(map[string]swarm.Chunk)
		ctx   = context.Background()
		err   error
	)

	for i := 0; i < 1000; i++ {
		ch := testingc.GenerateTestRandomChunk()
		if _, err = store.Put(ctx, ch); err != nil {
			t.Fatal(err)
		}
		chs[ch.Address().ByteString()] = ch
	}
	var wg sync.WaitGroup
	var o sync.Once
	var ccc = make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ccc
		ch := testingc.GenerateTestRandomChunk()
		if _, err = store.Put(ctx, ch); err != nil {
			t.Log(err)
		}
	}()

	err = store.Iterate(func(ch swarm.Chunk) (bool, error) {
		if !ch.Equal(chs[ch.Address().ByteString()]) {
			return true, errors.New("chunks not equal")
		}
		o.Do(func() {
			close(ccc)
		})
		time.Sleep(1 * time.Millisecond)
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	wg.Wait()

}

func BenchmarkSimpleStore_Simple(b *testing.B) {
	b.Skip()
	b.StopTimer()
	start := time.Now()
	const chunks = 10000 // 10k, 39mb
	ctx := context.Background()

	dir := b.TempDir()
	store, err := simplestore.New(dir)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	var chs []swarm.Address
	for i := 0; i < chunks; i++ {
		ch := testingc.GenerateTestRandomChunk()
		if _, err = store.Put(ctx, ch); err != nil {
			b.Fatal(err)
		}
		chs = append(chs, ch.Address())
	}
	fmt.Printf("took %s to insert %d chunks\n", time.Since(start), chunks)

	b.StartTimer()

	for n := 0; n < b.N; n++ {
		_, err := store.Get(ctx, chs[rand.Intn(len(chs))])
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSimpleStore_Multistore benchmarks how long it takes to retrieve a chunk
// given it might be on a certain store out of multiple stores.
// goos: linux
// goarch: amd64
// pkg: github.com/ethersphere/bee/pkg/storage/simplestore
// cpu: DO-Regular
// BenchmarkSimpleStore_Multistore
// BenchmarkSimpleStore_Multistore/1_stores,_5000_chunks_per_store
// BenchmarkSimpleStore_Multistore/1_stores,_5000_chunks_per_store-2         	   76722	     14922 ns/op
// BenchmarkSimpleStore_Multistore/1_stores,_10000_chunks_per_store
// BenchmarkSimpleStore_Multistore/1_stores,_10000_chunks_per_store-2        	   73534	     15928 ns/op
// BenchmarkSimpleStore_Multistore/1_stores,_20000_chunks_per_store
// BenchmarkSimpleStore_Multistore/1_stores,_20000_chunks_per_store-2        	   72916	     15677 ns/op
// BenchmarkSimpleStore_Multistore/5_stores,_5000_chunks_per_store
// BenchmarkSimpleStore_Multistore/5_stores,_5000_chunks_per_store-2         	   44803	     24622 ns/op
// BenchmarkSimpleStore_Multistore/5_stores,_10000_chunks_per_store
// BenchmarkSimpleStore_Multistore/5_stores,_10000_chunks_per_store-2        	   41581	     26806 ns/op
// BenchmarkSimpleStore_Multistore/5_stores,_20000_chunks_per_store
// BenchmarkSimpleStore_Multistore/5_stores,_20000_chunks_per_store-2        	   44026	     29421 ns/op
// BenchmarkSimpleStore_Multistore/10_stores,_5000_chunks_per_store
// BenchmarkSimpleStore_Multistore/10_stores,_5000_chunks_per_store-2        	   30938	     39738 ns/op
// BenchmarkSimpleStore_Multistore/10_stores,_10000_chunks_per_store
// BenchmarkSimpleStore_Multistore/10_stores,_10000_chunks_per_store-2       	   29566	     39527 ns/op
// BenchmarkSimpleStore_Multistore/10_stores,_20000_chunks_per_store
// BenchmarkSimpleStore_Multistore/10_stores,_20000_chunks_per_store-2       	   31870	     41503 ns/op
// PASS
// ok  	github.com/ethersphere/bee/pkg/storage/simplestore	8436.849s
func BenchmarkSimpleStore_Multistore(b *testing.B) {
	var (
		start         = time.Now()
		storeCount    = []int{1, 5, 10}
		chunks        = []int{5000, 10000, 20000} // chunks per store
		ctx           = context.Background()
		logInsertTime = false
	)

	for _, cnt := range storeCount {
		for _, chunksPerStore := range chunks {
			b.Run(fmt.Sprintf("%d stores, %d chunks per store", cnt, chunksPerStore), func(b *testing.B) {
				b.StopTimer()
				var (
					stores = make([]*simplestore.Store, cnt)
					err    error
					chs    []swarm.Address
				)
				for i := 0; i < cnt; i++ {
					dir := b.TempDir()
					stores[i], err = simplestore.New(dir)
					if err != nil {
						b.Fatal(err)
					}
					defer func(s *simplestore.Store) { s.Close() }(stores[i])
				}
				for i := 0; i < chunksPerStore*cnt; i++ {
					ch := testingc.GenerateTestRandomChunk()
					// pick a random store to put the chunk in
					if _, err = stores[rand.Intn(cnt)].Put(ctx, ch); err != nil {
						b.Fatal(err)
					}
					chs = append(chs, ch.Address())
				}
				if logInsertTime {
					fmt.Printf("took %s to insert %d chunks\n", time.Since(start), chunksPerStore*cnt)
				}
				b.StartTimer()

				for n := 0; n < b.N; n++ {
					get(ctx, b, chs[rand.Intn(len(chs))], stores)
				}
			})
		}
	}
}

func get(ctx context.Context, b *testing.B, addr swarm.Address, stores []*simplestore.Store) {
	b.Helper()
	for i := 0; i < len(stores); i++ {
		_, err := stores[i].Get(ctx, addr)
		if err == nil {
			return
		}
	}
	b.Fatal("chunk not found")
}

func testCount(t *testing.T, s *simplestore.Store, exp int) {
	t.Helper()
	cnt, err := s.Count()
	if err != nil {
		t.Fatal(err)
	}
	if cnt != exp {
		t.Fatalf("expected count %d got %d", exp, cnt)
	}
}

func newTestStore(t *testing.T) *simplestore.Store {
	t.Helper()
	dir := t.TempDir()
	store, err := simplestore.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}
