// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hashtrie_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"sync/atomic"
	"testing"

	bmtUtils "github.com/ethersphere/bee/v2/pkg/bmt"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/encryption"
	dec "github.com/ethersphere/bee/v2/pkg/encryption/store"
	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline/bmt"
	enc "github.com/ethersphere/bee/v2/pkg/file/pipeline/encryption"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline/hashtrie"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline/mock"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline/store"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var (
	addr swarm.Address
	span []byte
	ctx  = context.Background()
)

// nolint:gochecknoinits
func init() {
	b := make([]byte, 32)
	b[31] = 0x01
	addr = swarm.NewAddress(b)

	span = make([]byte, 8)
	binary.LittleEndian.PutUint64(span, 1)
}

// newErasureHashTrieWriter returns back an redundancy param and a HastTrieWriter pipeline
// which are using simple BMT and StoreWriter pipelines for chunk writes
func newErasureHashTrieWriter(
	ctx context.Context,
	s storage.Putter,
	rLevel redundancy.Level,
	encryptChunks bool,
	intermediateChunkPipeline, parityChunkPipeline pipeline.ChainWriter,
	replicaPutter storage.Putter,
) (redundancy.RedundancyParams, pipeline.ChainWriter) {
	pf := func() pipeline.ChainWriter {
		lsw := store.NewStoreWriter(ctx, s, intermediateChunkPipeline)
		return bmt.NewBmtWriter(lsw)
	}
	if encryptChunks {
		pf = func() pipeline.ChainWriter {
			lsw := store.NewStoreWriter(ctx, s, intermediateChunkPipeline)
			b := bmt.NewBmtWriter(lsw)
			return enc.NewEncryptionWriter(encryption.NewChunkEncrypter(), b)
		}
	}
	ppf := func() pipeline.ChainWriter {
		lsw := store.NewStoreWriter(ctx, s, parityChunkPipeline)
		return bmt.NewBmtWriter(lsw)
	}

	hashSize := swarm.HashSize
	if encryptChunks {
		hashSize *= 2
	}

	r := redundancy.New(rLevel, encryptChunks, ppf)
	ht := hashtrie.NewHashTrieWriter(ctx, hashSize, r, pf, replicaPutter)
	return r, ht
}

func TestLevels(t *testing.T) {
	t.Parallel()

	hashSize := 32

	// to create a level wrap we need to do branching^(level-1) writes
	for _, tc := range []struct {
		desc   string
		writes int
	}{
		{
			desc:   "2 at L1",
			writes: 2,
		},
		{
			desc:   "1 at L2, 1 at L1", // dangling chunk
			writes: 16 + 1,
		},
		{
			desc:   "1 at L3, 1 at L2, 1 at L1",
			writes: 64 + 16 + 1,
		},
		{
			desc:   "1 at L3, 2 at L2, 1 at L1",
			writes: 64 + 16 + 16 + 1,
		},
		{
			desc:   "1 at L5, 1 at L1",
			writes: 1024 + 1,
		},
		{
			desc:   "1 at L5, 1 at L3",
			writes: 1024 + 1,
		},
		{
			desc:   "2 at L5, 1 at L1",
			writes: 1024 + 1024 + 1,
		},
		{
			desc:   "3 at L5, 2 at L3, 1 at L1",
			writes: 1024 + 1024 + 1024 + 64 + 64 + 1,
		},
		{
			desc:   "1 at L7, 1 at L1",
			writes: 4096 + 1,
		},
		{
			desc:   "1 at L8", // balanced trie - all good
			writes: 16384,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()

			s := inmemchunkstore.New()
			pf := func() pipeline.ChainWriter {
				lsw := store.NewStoreWriter(ctx, s, nil)
				return bmt.NewBmtWriter(lsw)
			}

			ht := hashtrie.NewHashTrieWriter(ctx, hashSize, redundancy.New(0, false, pf), pf, s)

			for i := 0; i < tc.writes; i++ {
				a := &pipeline.PipeWriteArgs{Ref: addr.Bytes(), Span: span}
				err := ht.ChainWrite(a)
				if err != nil {
					t.Fatal(err)
				}
			}

			ref, err := ht.Sum()
			if err != nil {
				t.Fatal(err)
			}

			rootch, err := s.Get(ctx, swarm.NewAddress(ref))
			if err != nil {
				t.Fatal(err)
			}

			// check the span. since write spans are 1 value 1, then expected span == tc.writes
			sp := binary.LittleEndian.Uint64(rootch.Data()[:swarm.SpanSize])
			if sp != uint64(tc.writes) {
				t.Fatalf("want span %d got %d", tc.writes, sp)
			}
		})
	}
}

type redundancyMock struct {
	redundancy.Params
}

func (r redundancyMock) MaxShards() int {
	return 4
}

func TestLevels_TrieFull(t *testing.T) {
	t.Parallel()

	var (
		hashSize = 32
		writes   = 16384 // this is to get a balanced trie
		s        = inmemchunkstore.New()
		pf       = func() pipeline.ChainWriter {
			lsw := store.NewStoreWriter(ctx, s, nil)
			return bmt.NewBmtWriter(lsw)
		}
		r     = redundancy.New(0, false, pf)
		rMock = &redundancyMock{
			Params: *r,
		}

		ht = hashtrie.NewHashTrieWriter(ctx, hashSize, rMock, pf, s)
	)

	// to create a level wrap we need to do branching^(level-1) writes
	for i := 0; i < writes; i++ {
		a := &pipeline.PipeWriteArgs{Ref: addr.Bytes(), Span: span}
		err := ht.ChainWrite(a)
		if err != nil {
			t.Fatal(err)
		}
	}

	a := &pipeline.PipeWriteArgs{Ref: addr.Bytes(), Span: span}
	err := ht.ChainWrite(a)
	if !errors.Is(err, hashtrie.ErrTrieFull) {
		t.Fatal(err)
	}

	// it is questionable whether the writer should go into some
	// corrupt state after the last write which causes the trie full
	// error, in which case we would return an error on Sum()
	_, err = ht.Sum()
	if err != nil {
		t.Fatal(err)
	}
}

// TestRegression is a regression test for the bug
// described in https://github.com/ethersphere/bee/issues/1175
func TestRegression(t *testing.T) {
	t.Parallel()

	var (
		hashSize = 32
		writes   = 67100000 / 4096
		span     = make([]byte, 8)
		s        = inmemchunkstore.New()
		pf       = func() pipeline.ChainWriter {
			lsw := store.NewStoreWriter(ctx, s, nil)
			return bmt.NewBmtWriter(lsw)
		}
		ht = hashtrie.NewHashTrieWriter(ctx, hashSize, redundancy.New(0, false, pf), pf, s)
	)
	binary.LittleEndian.PutUint64(span, 4096)

	for i := 0; i < writes; i++ {
		a := &pipeline.PipeWriteArgs{Ref: addr.Bytes(), Span: span}
		err := ht.ChainWrite(a)
		if err != nil {
			t.Fatal(err)
		}
	}

	ref, err := ht.Sum()
	if err != nil {
		t.Fatal(err)
	}

	rootch, err := s.Get(ctx, swarm.NewAddress(ref))
	if err != nil {
		t.Fatal(err)
	}

	sp := binary.LittleEndian.Uint64(rootch.Data()[:swarm.SpanSize])
	if sp != uint64(writes*4096) {
		t.Fatalf("want span %d got %d", writes*4096, sp)
	}
}

type replicaPutter struct {
	storage.Putter
	replicaCount atomic.Uint32
}

func (r *replicaPutter) Put(ctx context.Context, chunk swarm.Chunk) error {
	r.replicaCount.Add(1)
	return r.Putter.Put(ctx, chunk)
}

// TestRedundancy using erasure coding library and checks carrierChunk function and modified span in intermediate chunk
func TestRedundancy(t *testing.T) {
	t.Parallel()
	// chunks need to have data so that it will not throw error on redundancy caching
	ch, err := cac.New(make([]byte, swarm.ChunkSize))
	if err != nil {
		t.Fatal(err)
	}
	chData := ch.Data()
	chSpan := chData[:swarm.SpanSize]
	chAddr := ch.Address().Bytes()

	// test logic assumes a simple 2 level chunk tree with carrier chunk
	for _, tc := range []struct {
		desc       string
		level      redundancy.Level
		encryption bool
		writes     int
		parities   int
	}{
		{
			desc:       "redundancy write for not encrypted data",
			level:      redundancy.INSANE,
			encryption: false,
			writes:     98, // 97 chunk references fit into one chunk + 1 carrier
			parities:   37, // 31 (full ch) + 6 (2 ref)
		},
		{
			desc:       "redundancy write for encrypted data",
			level:      redundancy.PARANOID,
			encryption: true,
			writes:     21,  // 21 encrypted chunk references fit into one chunk + 1 carrier
			parities:   116, // // 87 (full ch) + 29 (2 ref)
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			subCtx := redundancy.SetLevelInContext(ctx, tc.level)

			s := inmemchunkstore.New()
			intermediateChunkCounter := mock.NewChainWriter()
			parityChunkCounter := mock.NewChainWriter()
			replicaChunkCounter := &replicaPutter{Putter: s}

			r, ht := newErasureHashTrieWriter(subCtx, s, tc.level, tc.encryption, intermediateChunkCounter, parityChunkCounter, replicaChunkCounter)

			// write data to the hashTrie
			var key []byte
			if tc.encryption {
				key = make([]byte, swarm.HashSize)
			}
			for i := 0; i < tc.writes; i++ {
				a := &pipeline.PipeWriteArgs{Data: chData, Span: chSpan, Ref: chAddr, Key: key}
				err := ht.ChainWrite(a)
				if err != nil {
					t.Fatal(err)
				}
			}

			ref, err := ht.Sum()
			if err != nil {
				t.Fatal(err)
			}

			// sanity check for the test samples
			if tc.parities != parityChunkCounter.ChainWriteCalls() {
				t.Errorf("generated parities should be %d. Got: %d", tc.parities, parityChunkCounter.ChainWriteCalls())
			}
			if intermediateChunkCounter.ChainWriteCalls() != 2 { // root chunk and the chunk which was written before carrierChunk movement
				t.Errorf("effective chunks should be %d. Got: %d", tc.writes, intermediateChunkCounter.ChainWriteCalls())
			}

			rootch, err := s.Get(subCtx, swarm.NewAddress(ref[:swarm.HashSize]))
			if err != nil {
				t.Fatal(err)
			}
			chData := rootch.Data()
			if tc.encryption {
				chData, err = dec.DecryptChunkData(chData, ref[swarm.HashSize:])
				if err != nil {
					t.Fatal(err)
				}
			}

			// span check
			level, sp := redundancy.DecodeSpan(chData[:swarm.SpanSize])
			expectedSpan := bmtUtils.LengthToSpan(int64(tc.writes * swarm.ChunkSize))
			if !bytes.Equal(expectedSpan, sp) {
				t.Fatalf("want span %d got %d", expectedSpan, span)
			}
			if level != tc.level {
				t.Fatalf("encoded level differs from the uploaded one %d. Got: %d", tc.level, level)
			}
			expectedParities := tc.parities - r.Parities(r.MaxShards())
			_, parity := file.ReferenceCount(bmtUtils.LengthFromSpan(sp), level, tc.encryption)
			if expectedParities != parity {
				t.Fatalf("want parity %d got %d", expectedParities, parity)
			}
			if tc.level.GetReplicaCount() != int(replicaChunkCounter.replicaCount.Load()) {
				t.Fatalf("unexpected number of replicas: want %d. Got: %d", tc.level.GetReplicaCount(), int(replicaChunkCounter.replicaCount.Load()))
			}
		})
	}
}
