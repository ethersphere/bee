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

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/file/redundancy/getter"
	"github.com/ethersphere/bee/pkg/storage"
	inmem "github.com/ethersphere/bee/pkg/storage/inmemchunkstore"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/klauspost/reedsolomon"
)

// TestGetter tests the retrieval of chunks with missing data shards
// using the RACE strategy for a number of erasure code parameters
func TestGetterRACE(t *testing.T) {
	// reset retry interval to speed up tests
	retryInterval := getter.RetryInterval
	defer func() { getter.RetryInterval = retryInterval }()
	getter.RetryInterval = 30 * time.Millisecond

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

// testDecodingFallback tests the retrieval of chunks with missing data shards
func testDecodingFallback(t *testing.T, s getter.Strategy, strict bool) {

	retryInterval := getter.RetryInterval
	defer func() { getter.RetryInterval = retryInterval }()
	getter.RetryInterval = 30 * time.Millisecond

	strategyTimeout := getter.StrategyTimeout
	defer func() { getter.StrategyTimeout = strategyTimeout }()
	getter.StrategyTimeout = 100 * time.Millisecond

	bufSize := 12
	shardCnt := 6
	store := inmem.New()
	buf := make([][]byte, bufSize)
	addrs := initData(t, buf, shardCnt, store)

	// erase two data shards
	delayed, erased := 1, 0
	ctx := context.TODO()
	for _, i := range []int{delayed, erased} {
		err := store.Delete(ctx, addrs[i])
		if err != nil {
			t.Fatal(err)
		}
	}

	// context for enforced retrievals with long timeout
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	// signal channels for delayed and erased chunk retrieval
	waitDelayed, waitErased := make(chan error, 1), make(chan error, 1)

	// complete retrieval of delayed chunk by putting it into the store after a while
	delay := getter.RetryInterval + 10*time.Millisecond
	if s == getter.NONE {
		delay += getter.StrategyTimeout
	}

	// create getter
	start := time.Now()
	g := getter.New(addrs, shardCnt, store, store, s, strict)
	defer g.Close()

	// launch delayed and erased chunk retrieval
	wg := sync.WaitGroup{}
	defer wg.Wait()
	wg.Add(3)
	go func() {
		defer wg.Done()
		time.Sleep(delay)
		_ = store.Put(ctx, swarm.NewChunk(addrs[delayed], buf[delayed]))
	}()
	// signal using the waitDelayed and waitErased channels when
	// delayed and erased chunk retrieval completes
	go func() {
		defer wg.Done()
		_, err := g.Get(ctx, addrs[delayed])
		waitDelayed <- err
	}()
	go func() {
		defer wg.Done()
		_, err := g.Get(ctx, addrs[erased])
		waitErased <- err
	}()

	// set timeouts for the cases
	var timeout time.Duration
	switch {
	case strict:
		timeout = 2*getter.StrategyTimeout - 10*time.Millisecond
	case s == getter.NONE:
		timeout = 4*getter.StrategyTimeout - 10*time.Millisecond
	case s == getter.DATA:
		timeout = 3*getter.StrategyTimeout - 10*time.Millisecond
	}

	// wait for delayed chunk retrieval to complete
	select {
	case err := <-waitDelayed:
		if err != nil {
			t.Fatal("unexpected error", err)
		}
		switch {
		case strict && s == getter.NONE:
			if time.Since(start)/getter.StrategyTimeout < 1 {
				t.Fatal("unexpected completion of delayed chunk retrieval")
			}
		case s == getter.NONE:
			if time.Since(start)/getter.StrategyTimeout < 1 {
				t.Fatal("unexpected early completion of delayed chunk retrieval")
			}
		case s == getter.DATA:
		}

		if time.Since(start)/getter.StrategyTimeout > 1 {
			t.Fatal("unexpected late completion of delayed chunk retrieval")
		}
		checkShardsAvailable(t, store, addrs[delayed:], buf[delayed:])
		// wait for erased chunk retrieval to complete
		select {
		case err := <-waitErased:
			if err != nil {
				t.Fatal("unexpected error", err)
			}
			switch {
			case strict:
				t.Fatal("unexpected completion of delayed chunk retrieval")
			case s == getter.NONE:
				if time.Since(start)/getter.StrategyTimeout < 1 {
					t.Fatal("unexpected early completion of delayed chunk retrieval")
				}
			case s == getter.DATA:
				if time.Since(start)/getter.StrategyTimeout > 1 {
					t.Fatal("unexpected late completion of delayed chunk retrieval")
				}
			}
			checkShardsAvailable(t, store, addrs[:erased], buf[:erased])

		case <-time.After(300 * time.Millisecond):
			if !strict {
				t.Fatal("unexpected timeout using strategy", s, "with strict", strict)
			}
		}
	case <-time.After(timeout):
		if !strict || s != getter.NONE {
			t.Fatal("unexpected timeout using strategy", s, "with strict", strict)
		}
	}
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
	g := getter.New(addrs, shardCnt, store, store, getter.RACE, true)
	defer g.Close()
	parityCnt := len(buf) - shardCnt

	ctx, cancel := context.WithTimeout(context.TODO(), 150*time.Millisecond)
	defer cancel()
	_, err := g.Get(ctx, addr)

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
	ctx := context.TODO()
	for i, addr := range addrs {
		ch, err := s.Get(ctx, addr)
		if err != nil {
			t.Fatalf("datashard %d with address %v is not available: %v", i, addr, err)
		}
		if !bytes.Equal(ch.Data(), data[i]) {
			t.Fatalf("datashard %d has incorrect data", i)
		}
	}
}

func forget(t *testing.T, store storage.ChunkStore, addrs []swarm.Address, erasureCnt int) (erasures []int) {
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
