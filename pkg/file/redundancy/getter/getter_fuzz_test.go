// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package getter_test

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy/getter"
	"github.com/ethersphere/bee/v2/pkg/log"
	inmem "github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzGetterDecode drives fuzzed chunk bytes through the real Reed-Solomon
// recovery path of the redundancy getter (prefetch -> runStrategy -> decode ->
// recover, i.e. reedsolomon.ReconstructData plus the setData padding and
// getData lastLen slicing). Erasure-coded intermediate chunks are fetched from
// the network/local store, so their bytes and the erasure pattern are
// effectively attacker-controlled: the getter must never panic on
// RS-inconsistent or malformed shards, regardless of which shards are missing.
// It must instead return a normal error (ErrNotFound / deadline exceeded).
func FuzzGetterDecode(f *testing.F) {
	f.Add([]byte("seed chunk data"), byte(0), byte(3), uint16(0b10))
	f.Add(make([]byte, swarm.ChunkWithSpanSize), byte(5), byte(1), uint16(0xffff))
	f.Add([]byte{}, byte(2), byte(0), uint16(0))

	f.Fuzz(func(t *testing.T, data []byte, bufSel, shardSel byte, eraseMask uint16) {
		// bounded buffer so the concurrent prefetch cannot blow up memory or
		// spawn an unbounded number of goroutines across fuzz iterations.
		bufSize := 3 + int(bufSel)%14 // 3..16
		shardCnt := 1 + int(shardSel)%(bufSize-1)

		store := inmem.New()

		span := make([]byte, swarm.SpanSize)
		binary.LittleEndian.PutUint64(span, swarm.ChunkSize)

		addrs := make([]swarm.Address, bufSize)
		for i := range addrs {
			// build a valid CAC over fuzzed bytes; vary a byte per index so the
			// chunk addresses stay distinct (the cache/wait maps are keyed by them).
			cdata := make([]byte, swarm.ChunkWithSpanSize)
			copy(cdata, span)
			copy(cdata[swarm.SpanSize:], data)
			cdata[swarm.SpanSize] ^= byte(i)

			ch, err := cac.NewWithDataSpan(cdata)
			if err != nil {
				t.Fatalf("build chunk: %v", err) // fixed-size input must always be valid
			}
			if err := store.Put(context.Background(), ch); err != nil {
				t.Fatalf("put chunk: %v", err)
			}
			addrs[i] = ch.Address()
		}

		// erase a fuzz-selected subset to force the recovery path, but never the
		// target data shard (addrs[0]) so Get has something to look up.
		for i := 1; i < bufSize && i < 16; i++ {
			if eraseMask&(1<<uint(i)) != 0 {
				if err := store.Delete(context.Background(), addrs[i]); err != nil {
					t.Fatalf("delete chunk: %v", err)
				}
			}
		}

		g := getter.New(addrs, shardCnt, store, store, func(error) {}, getter.Config{
			Strategy:     getter.RACE,
			Strict:       true,
			FetchTimeout: 5 * time.Millisecond,
			Logger:       log.Noop,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		// must not panic for any fuzzed bytes / erasure pattern.
		ch, err := g.Get(ctx, addrs[0])
		if err == nil {
			// success-only invariant: a non-nil chunk for the requested address.
			if ch == nil {
				t.Fatal("nil error but nil chunk")
			}
			if !ch.Address().Equal(addrs[0]) {
				t.Fatalf("returned chunk address %s != requested %s", ch.Address(), addrs[0])
			}
		}
	})
}
