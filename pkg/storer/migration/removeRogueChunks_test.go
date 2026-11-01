// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/storage"
	chunktest "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/stampindex"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	localmigration "github.com/ethersphere/bee/v2/pkg/storer/migration"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestRemoveRogueChunks(t *testing.T) {
	t.Parallel()

	store := internal.NewInmemStorage()

	validCAC := chunktest.GenerateTestRandomChunk()
	validSOC := chunktest.GenerateTestRandomSoChunk(t, chunktest.GenerateTestRandomChunk())
	// a correctly signed SOC delivered under an address that is not its own
	misaddressedSOC := swarm.NewChunk(swarm.RandAddress(t), validSOC.Data()).WithStamp(postagetesting.MustNewStamp())
	// the same SOC data under another wrong address, stored as an SOC because it
	// was accepted before SOC validation checked the address
	acceptedMisaddressedSOC := swarm.NewChunk(swarm.RandAddress(t), validSOC.Data()).WithStamp(postagetesting.MustNewStamp())
	invalid := chunktest.GenerateTestRandomInvalidChunk()
	mistypedCAC := chunktest.GenerateTestRandomChunk()
	missing := chunktest.GenerateTestRandomInvalidChunk()

	tcs := []struct {
		name       string
		chunk      swarm.Chunk
		storedType swarm.ChunkType
		inStore    bool
		wantType   swarm.ChunkType // unspecified means removed
	}{
		{"valid cac", validCAC, swarm.ChunkTypeContentAddressed, true, swarm.ChunkTypeContentAddressed},
		{"valid soc", validSOC, swarm.ChunkTypeSingleOwner, true, swarm.ChunkTypeSingleOwner},
		{"misaddressed soc", misaddressedSOC, swarm.ChunkTypeUnspecified, true, swarm.ChunkTypeUnspecified},
		{"accepted misaddressed soc", acceptedMisaddressedSOC, swarm.ChunkTypeSingleOwner, true, swarm.ChunkTypeUnspecified},
		{"invalid chunk", invalid, swarm.ChunkTypeUnspecified, true, swarm.ChunkTypeUnspecified},
		{"mistyped cac", mistypedCAC, swarm.ChunkTypeUnspecified, true, swarm.ChunkTypeContentAddressed},
		{"missing chunk", missing, swarm.ChunkTypeUnspecified, false, swarm.ChunkTypeUnspecified},
	}

	items := make([]*reserve.ChunkBinItem, len(tcs))
	for i, tc := range tcs {
		stampHash, err := tc.chunk.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}
		items[i] = &reserve.ChunkBinItem{
			Bin:       0,
			BinID:     uint64(i + 1),
			Address:   tc.chunk.Address(),
			BatchID:   tc.chunk.Stamp().BatchID(),
			ChunkType: tc.storedType,
			StampHash: stampHash,
		}
		err = store.Run(context.Background(), func(s transaction.Store) error {
			err := errors.Join(
				s.IndexStore().Put(items[i]),
				s.IndexStore().Put(batchRadiusItem(items[i])),
				chunkstamp.Store(s.IndexStore(), "reserve", tc.chunk),
				stampindex.Store(s.IndexStore(), "reserve", tc.chunk),
			)
			if err != nil || !tc.inStore {
				return err
			}
			return s.ChunkStore().Put(context.Background(), tc.chunk)
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if err := localmigration.RemoveRogueChunks(store, log.Noop)(); err != nil {
		t.Fatal(err)
	}

	for i, tc := range tcs {
		item := items[i]
		removed := tc.wantType == swarm.ChunkTypeUnspecified

		got := &reserve.ChunkBinItem{Bin: item.Bin, BinID: item.BinID}
		err := store.IndexStore().Get(got)
		switch {
		case removed && !errors.Is(err, storage.ErrNotFound):
			t.Errorf("%s: chunk bin item: want not found, got %v", tc.name, err)
		case !removed && err != nil:
			t.Errorf("%s: chunk bin item: %v", tc.name, err)
		case !removed && got.ChunkType != tc.wantType:
			t.Errorf("%s: chunk type: want %v, got %v", tc.name, tc.wantType, got.ChunkType)
		}

		has, err := store.IndexStore().Has(batchRadiusItem(item))
		if err != nil {
			t.Fatal(err)
		}
		if has == removed {
			t.Errorf("%s: batch radius item present: %t", tc.name, has)
		}

		_, err = chunkstamp.LoadWithStampHash(store.IndexStore(), "reserve", item.Address, item.StampHash)
		if removed != errors.Is(err, storage.ErrNotFound) {
			t.Errorf("%s: chunkstamp: %v", tc.name, err)
		}

		_, err = stampindex.Load(store.IndexStore(), "reserve", tc.chunk.Stamp())
		if removed != errors.Is(err, storage.ErrNotFound) {
			t.Errorf("%s: stampindex: %v", tc.name, err)
		}

		has, err = store.ChunkStore().Has(context.Background(), item.Address)
		if err != nil {
			t.Fatal(err)
		}
		if has == removed {
			t.Errorf("%s: chunk present in chunkstore: %t", tc.name, has)
		}
	}
}

func batchRadiusItem(item *reserve.ChunkBinItem) *reserve.BatchRadiusItem {
	return &reserve.BatchRadiusItem{
		Bin:       item.Bin,
		BatchID:   item.BatchID,
		Address:   item.Address,
		BinID:     item.BinID,
		StampHash: item.StampHash,
	}
}
