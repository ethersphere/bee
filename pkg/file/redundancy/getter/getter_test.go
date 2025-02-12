// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package getter_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	mrand "math/rand"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy/getter"
	"github.com/ethersphere/bee/v2/pkg/storage"
	inmem "github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/klauspost/reedsolomon"
	"golang.org/x/sync/errgroup"
)

// TestGetter tests the retrieval of chunks with missing data shards
// using the RACE strategy for a number of erasure code parameters
func TestGetterRACE_FLAKY(t *testing.T) {
	type getterTest struct {
		bufSize    int
		shardCnt   int
		erasureCnt int
	}

	var tcs []getterTest
	for bufSize := 3; bufSize <= 128; bufSize += 21 {
		for shardCnt := bufSize/2 + 1; shardCnt <= bufSize; shardCnt += 21 {
			parityCnt := bufSize - shardCnt
			erasures := mrand.Perm(parityCnt - 1)
			if len(erasures) > 3 {
				erasures = erasures[:3]
			}
			for _, erasureCnt := range erasures {
				tcs = append(tcs, getterTest{bufSize, shardCnt, erasureCnt})
			}
			tcs = append(tcs, getterTest{bufSize, shardCnt, parityCnt}, getterTest{bufSize, shardCnt, parityCnt + 1})
			erasures = mrand.Perm(shardCnt - 1)
			if len(erasures) > 3 {
				erasures = erasures[:3]
			}
			for _, erasureCnt := range erasures {
				tcs = append(tcs, getterTest{bufSize, shardCnt, erasureCnt + parityCnt + 1})
			}
		}
	}
	t.Run("GET with RACE", func(t *testing.T) {
		t.Parallel()

		for _, tc := range tcs {
			t.Run(fmt.Sprintf("data/total/missing=%d/%d/%d", tc.shardCnt, tc.bufSize, tc.erasureCnt), func(t *testing.T) {
				testDecodingRACE(t, tc.bufSize, tc.shardCnt, tc.erasureCnt)
			})
		}
	})
}

// TestGetterFallback tests the retrieval of chunks with missing data shards
// using the strict or fallback mode starting with NONE and DATA strategies
func TestGetterFallback(t *testing.T) {
	t.Skip("removed strategy timeout")
	t.Run("GET", func(t *testing.T) {
		t.Run("NONE", func(t *testing.T) {
			t.Run("strict", func(t *testing.T) {
				testDecodingFallback(t, getter.NONE, true)
			})
			t.Run("fallback", func(t *testing.T) {
				testDecodingFallback(t, getter.NONE, false)
			})
		})
		t.Run("DATA", func(t *testing.T) {
			t.Run("strict", func(t *testing.T) {
				testDecodingFallback(t, getter.DATA, true)
			})
			t.Run("fallback", func(t *testing.T) {
				testDecodingFallback(t, getter.DATA, false)
			})
		})
	})
}

func testDecodingRACE(t *testing.T, bufSize, shardCnt, erasureCnt int) {
	t.Helper()
	store := inmem.New()
	buf := make([][]byte, bufSize)
	addrs := initData(t, buf, shardCnt, store)

	var addr swarm.Address
	erasures := forget(t, store, addrs, erasureCnt)
	for _, i := range erasures {
		if i < shardCnt {
			addr = addrs[i]
			break
		}
	}
	if len(addr.Bytes()) == 0 {
		t.Skip("no data shard erased")
	}

	g := getter.New(addrs, shardCnt, store, store, func(error) {}, getter.DefaultConfig)

	parityCnt := len(buf) - shardCnt
	_, err := g.Get(context.Background(), addr)

	switch {
	case erasureCnt > parityCnt:
		t.Run("unable to recover", func(t *testing.T) {
			if !errors.Is(err, storage.ErrNotFound) &&
				!errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("expected not found error or deadline exceeded, got %v", err)
			}
		})
	case erasureCnt <= parityCnt:
		t.Run("will recover", func(t *testing.T) {
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			checkShardsAvailable(t, store, addrs[:shardCnt], buf[:shardCnt])
		})
	}
}

// testDecodingFallback tests the retrieval of chunks with missing data shards
func testDecodingFallback(t *testing.T, s getter.Strategy, strict bool) {
	t.Helper()

	strategyTimeout := 150 * time.Millisecond

	bufSize := 12
	shardCnt := 6
	store := mockstorer.NewDelayedStore(inmem.New())
	buf := make([][]byte, bufSize)
	addrs := initData(t, buf, shardCnt, store)

	// erase two data shards
	delayed, erased := 1, 0
	ctx := context.TODO()
	err := store.Delete(ctx, addrs[erased])
	if err != nil {
		t.Fatal(err)
	}

	// context for enforced retrievals with long timeout
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// signal channels for delayed and erased chunk retrieval
	waitDelayed, waitErased := make(chan error, 1), make(chan error, 1)

	// complete retrieval of delayed chunk by putting it into the store after a while
	delay := strategyTimeout / 4
	if s == getter.NONE {
		delay += strategyTimeout
	}
	store.Delay(addrs[delayed], delay)
	// create getter
	start := time.Now()
	conf := getter.Config{
		Strategy:     s,
		Strict:       strict,
		FetchTimeout: strategyTimeout / 2,
	}
	g := getter.New(addrs, shardCnt, store, store, func(error) {}, conf)

	// launch delayed and erased chunk retrieval
	wg := sync.WaitGroup{}
	// defer wg.Wait()
	wg.Add(2)
	// signal using the waitDelayed and waitErased channels when
	// delayed and erased chunk retrieval completes
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(ctx, strategyTimeout*time.Duration(5-s))
		defer cancel()
		_, err := g.Get(ctx, addrs[delayed])
		waitDelayed <- err
	}()
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(ctx, strategyTimeout*time.Duration(5-s))
		defer cancel()
		_, err := g.Get(ctx, addrs[erased])
		waitErased <- err
	}()

	// wait for delayed chunk retrieval to complete
	select {
	case err := <-waitDelayed:
		if err != nil {
			t.Fatal("unexpected error", err)
		}
		round := time.Since(start) / strategyTimeout
		switch {
		case strict && s == getter.NONE:
			if round < 1 {
				t.Fatalf("unexpected completion of delayed chunk retrieval. got round %d", round)
			}
		case s == getter.NONE:
			if round < 1 {
				t.Fatalf("unexpected completion of delayed chunk retrieval. got round %d", round)
			}
			if round > 2 {
				t.Fatalf("unexpected late completion of delayed chunk retrieval. got round %d", round)
			}
		case s == getter.DATA:
			if round > 0 {
				t.Fatalf("unexpected late completion of delayed chunk retrieval. got round %d", round)
			}
		}

		checkShardsAvailable(t, store, addrs[delayed:], buf[delayed:])
		// wait for erased chunk retrieval to complete
		select {
		case err := <-waitErased:
			if err != nil {
				t.Fatal("unexpected error", err)
			}
			round = time.Since(start) / strategyTimeout
			switch {
			case strict:
				t.Fatalf("unexpected completion of erased chunk retrieval. got round %d", round)
			case s == getter.NONE:
				if round < 3 {
					t.Fatalf("unexpected early completion of erased chunk retrieval. got round %d", round)
				}
				if round > 3 {
					t.Fatalf("unexpected late completion of erased chunk retrieval. got round %d", round)
				}
			case s == getter.DATA:
				if round < 1 {
					t.Fatalf("unexpected early completion of erased chunk retrieval. got round %d", round)
				}
				if round > 1 {
					t.Fatalf("unexpected late completion of delayed chunk retrieval. got round %d", round)
				}
			}
			checkShardsAvailable(t, store, addrs[:erased], buf[:erased])

		case <-time.After(strategyTimeout * 2):
			if !strict {
				t.Fatal("unexpected timeout using strategy", s, "with strict", strict)
			}
		}
	case <-time.After(strategyTimeout * 3):
		if !strict || s != getter.NONE {
			t.Fatal("unexpected timeout using strategy", s, "with strict", strict)
		}
	}
}

func initData(t *testing.T, buf [][]byte, shardCnt int, s storage.ChunkStore) []swarm.Address {
	t.Helper()
	spanBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(spanBytes, swarm.ChunkSize)

	for i := 0; i < len(buf); i++ {
		buf[i] = make([]byte, swarm.ChunkWithSpanSize)
		if i >= shardCnt {
			continue
		}
		_, err := io.ReadFull(rand.Reader, buf[i])
		if err != nil {
			t.Fatal(err)
		}
		copy(buf[i], spanBytes)
	}

	// fill in parity chunks
	rs, err := reedsolomon.New(shardCnt, len(buf)-shardCnt)
	if err != nil {
		t.Fatal(err)
	}
	err = rs.Encode(buf)
	if err != nil {
		t.Fatal(err)
	}

	// calculate chunk addresses and upload to the store
	addrs := make([]swarm.Address, len(buf))
	ctx := context.TODO()
	for i := 0; i < len(buf); i++ {
		chunk, err := cac.NewWithDataSpan(buf[i])
		if err != nil {
			t.Fatal(err)
		}
		err = s.Put(ctx, chunk)
		if err != nil {
			t.Fatal(err)
		}
		addrs[i] = chunk.Address()
	}

	return addrs
}

func checkShardsAvailable(t *testing.T, s storage.ChunkStore, addrs []swarm.Address, data [][]byte) {
	t.Helper()
	eg, ctx := errgroup.WithContext(context.Background())
	for i, addr := range addrs {
		eg.Go(func() (err error) {
			var delay time.Duration
			var ch swarm.Chunk
			for i := 0; i < 30; i++ {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					<-time.After(delay)
					delay = 50 * time.Millisecond
				}
				ch, err = s.Get(ctx, addr)
				if err == nil {
					break
				}
				err = fmt.Errorf("datashard %d with address %v is not available: %w", i, addr, err)
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					<-time.After(delay)
					delay = 50 * time.Millisecond
				}
			}
			if err == nil && !bytes.Equal(ch.Data(), data[i]) {
				return fmt.Errorf("datashard %d has incorrect data", i)
			}
			return err
		})
	}
	if err := eg.Wait(); err != nil {
		t.Fatal(err)
	}
}

func forget(t *testing.T, store storage.ChunkStore, addrs []swarm.Address, erasureCnt int) (erasures []int) {
	t.Helper()

	ctx := context.TODO()
	erasures = mrand.Perm(len(addrs))[:erasureCnt]
	for _, i := range erasures {
		err := store.Delete(ctx, addrs[i])
		if err != nil {
			t.Fatal(err)
		}
	}
	return erasures
}
