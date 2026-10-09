// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"path"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/log"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	soctesting "github.com/ethersphere/bee/v2/pkg/soc/testing"
	"github.com/ethersphere/bee/v2/pkg/storage"
	chunktest "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	localmigration "github.com/ethersphere/bee/v2/pkg/storer/migration"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// legacyChunkBinItem writes the pre-Sum, 106-byte ChunkBinItem serialization
// under the chunkBin namespace, byte-compatible with the on-disk format the
// migration upgrades from.
type legacyChunkBinItem struct {
	bin       uint8
	binID     uint64
	address   swarm.Address
	batchID   []byte
	chunkType swarm.ChunkType
	stampHash []byte
}

func (l *legacyChunkBinItem) Namespace() string { return "chunkBin" }

func (l *legacyChunkBinItem) ID() string {
	binIDBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(binIDBytes, l.binID)
	return string(l.bin) + string(binIDBytes)
}

func (l *legacyChunkBinItem) String() string { return path.Join(l.Namespace(), l.ID()) }

func (l *legacyChunkBinItem) Marshal() ([]byte, error) {
	buf := make([]byte, 1+8+swarm.HashSize+swarm.HashSize+1+swarm.HashSize)
	i := 0
	buf[i] = l.bin
	i += 1
	binary.BigEndian.PutUint64(buf[i:i+8], l.binID)
	i += 8
	copy(buf[i:i+swarm.HashSize], l.address.Bytes())
	i += swarm.HashSize
	copy(buf[i:i+swarm.HashSize], l.batchID)
	i += swarm.HashSize
	buf[i] = uint8(l.chunkType)
	i += 1
	copy(buf[i:i+swarm.HashSize], l.stampHash)
	return buf, nil
}

func (l *legacyChunkBinItem) Unmarshal(_ []byte) error { return errors.New("not expected") }

func (l *legacyChunkBinItem) Clone() storage.Item {
	c := *l
	return &c
}

// TestStep08 runs the Sum backfill migration against genuine pre-migration
// records: old-format ChunkBinItems, entries whose chunk is missing from the
// chunkstore, an orphaned old-format ChunkBinItem and a legacy entry with an
// unset stamp hash. The migration must upgrade the healthy entries and remove
// every other one without failing.
func TestStep08(t *testing.T) {
	t.Parallel()

	store := internal.NewInmemStorage()
	baseAddr := swarm.RandAddress(t)
	ctx := context.Background()

	binID := uint64(0)
	nextBinID := func() uint64 { binID++; return binID }

	// seed writes a full pre-migration reserve entry for the chunk. When
	// inChunkstore is false the chunk data and stamp are omitted, simulating a
	// dangling index entry.
	seed := func(ch swarm.Chunk, stampHash []byte, inChunkstore bool) *reserve.BatchRadiusItem {
		t.Helper()
		bin := swarm.Proximity(baseAddr.Bytes(), ch.Address().Bytes())
		id := nextBinID()
		br := &reserve.BatchRadiusItem{
			Bin:       bin,
			BatchID:   ch.Stamp().BatchID(),
			Address:   ch.Address(),
			BinID:     id,
			StampHash: stampHash,
		}
		err := store.Run(ctx, func(s transaction.Store) error {
			err := errors.Join(
				s.IndexStore().Put(br),
				s.IndexStore().Put(&legacyChunkBinItem{
					bin:       bin,
					binID:     id,
					address:   ch.Address(),
					batchID:   ch.Stamp().BatchID(),
					chunkType: storage.ChunkType(ch),
					stampHash: stampHash,
				}),
			)
			if err != nil || !inChunkstore {
				return err
			}
			return errors.Join(
				chunkstamp.Store(s.IndexStore(), "reserve", ch),
				s.ChunkStore().Put(ctx, ch),
			)
		})
		if err != nil {
			t.Fatal(err)
		}
		return br
	}

	stampHashOf := func(ch swarm.Chunk) []byte {
		t.Helper()
		h, err := ch.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}
		return h
	}

	// newValidCAC builds a genuine content addressed chunk: the migration
	// classifies chunks with the real ChunkType, so random-data chunks would
	// be removed as invalid.
	newValidCAC := func(data string) swarm.Chunk {
		t.Helper()
		ch, err := cac.New([]byte(data))
		if err != nil {
			t.Fatal(err)
		}
		return ch.WithStamp(postagetesting.MustNewStamp())
	}

	// healthy CAC entry
	cacCh := newValidCAC("healthy cac")
	cacBr := seed(cacCh, stampHashOf(cacCh), true)

	// healthy SOC entry: its sum must fold the wrapped CAC address
	socChunk := soctesting.GenerateMockSOC(t, []byte("payload")).
		Chunk().WithStamp(postagetesting.MustNewStamp())
	socBr := seed(socChunk, stampHashOf(socChunk), true)

	// dangling entry: chunk missing from the chunkstore. Its removal exercises
	// deleteChunkBinItem against the old-format record.
	dangling := chunktest.GenerateTestRandomChunkAt(t, baseAddr, 1)
	danglingBr := seed(dangling, stampHashOf(dangling), false)

	// legacy entry with an unset stamp hash: the chunk itself is valid, so
	// its removal is attributable to the stamp hash check alone
	zeroHash := newValidCAC("zero stamp hash")
	zeroHashBr := seed(zeroHash, swarm.EmptyAddress.Bytes(), true)

	// orphaned old-format ChunkBinItem: no BatchRadiusItem, so the backfill
	// never rewrites it and only the trailing sweep can remove it.
	orphan := chunktest.GenerateTestRandomChunkAt(t, baseAddr, 1)
	orphanItem := &legacyChunkBinItem{
		bin:       swarm.Proximity(baseAddr.Bytes(), orphan.Address().Bytes()),
		binID:     nextBinID(),
		address:   orphan.Address(),
		batchID:   orphan.Stamp().BatchID(),
		chunkType: swarm.ChunkTypeContentAddressed,
		stampHash: stampHashOf(orphan),
	}
	if err := store.Run(ctx, func(s transaction.Store) error {
		return s.IndexStore().Put(orphanItem)
	}); err != nil {
		t.Fatal(err)
	}

	if err := localmigration.Step08(store, log.Noop)(); err != nil {
		t.Fatal(err)
	}

	// the healthy entries are upgraded in place with correct sums in both
	// indexes.
	for _, tc := range []struct {
		ch swarm.Chunk
		br *reserve.BatchRadiusItem
	}{
		{cacCh, cacBr},
		{socChunk, socBr},
	} {
		sum, err := storage.ChunkSum(tc.ch)
		if err != nil {
			t.Fatal(err)
		}
		cbi := &reserve.ChunkBinItem{Bin: tc.br.Bin, BinID: tc.br.BinID}
		if err := store.IndexStore().Get(cbi); err != nil {
			t.Fatalf("expected upgraded chunk bin item for %s: %v", tc.ch.Address(), err)
		}
		if !bytes.Equal(cbi.Sum, sum) {
			t.Fatalf("wrong sum on upgraded chunk bin item for %s", tc.ch.Address())
		}
		has, err := store.IndexStore().Has(&reserve.ChunkSumItem{Address: tc.ch.Address(), Sum: sum})
		if err != nil {
			t.Fatal(err)
		}
		if !has {
			t.Fatalf("expected chunk sum item for %s", tc.ch.Address())
		}
	}

	// the dangling and zero-stamp-hash entries are removed entirely.
	for _, br := range []*reserve.BatchRadiusItem{danglingBr, zeroHashBr} {
		has, err := store.IndexStore().Has(br)
		if err != nil {
			t.Fatal(err)
		}
		if has {
			t.Fatalf("expected batch radius item for %s to be removed", br.Address)
		}
	}

	// no chunkBin record of any format survives except the two upgraded ones,
	// proving the orphan was swept and every remaining value unmarshals.
	count := 0
	err := store.IndexStore().Iterate(
		storage.Query{Factory: func() storage.Item { return &reserve.ChunkBinItem{} }},
		func(res storage.Result) (bool, error) {
			count++
			return false, nil
		},
	)
	if err != nil {
		t.Fatalf("chunkBin namespace must be fully decodable after migration: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 chunk bin items after migration, got %d", count)
	}
}
