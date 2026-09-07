// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/log"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/sharky"
	soctesting "github.com/ethersphere/bee/v2/pkg/soc/testing"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/stampindex"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	localmigration "github.com/ethersphere/bee/v2/pkg/storer/migration"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	kademlia "github.com/ethersphere/bee/v2/pkg/topology/mock"
)

type dirFS struct{ basedir string }

func (d *dirFS) Open(p string) (fs.File, error) {
	return os.OpenFile(filepath.Join(d.basedir, p), os.O_RDWR|os.O_CREATE, 0o600)
}

// newRealStorage builds the production storage stack (sharky blobs, leveldb
// index, batched transactions, reference-counted chunkstore). The in-memory
// test storage commits every write immediately and does not count references,
// so it cannot exhibit the failures covered here.
func newRealStorage(t *testing.T) transaction.Storage {
	t.Helper()
	sh, err := sharky.New(&dirFS{basedir: t.TempDir()}, 32, swarm.SocMaxChunkSize)
	if err != nil {
		t.Fatal(err)
	}
	ldb, _, err := leveldbstore.New("", nil)
	if err != nil {
		t.Fatal(err)
	}
	st := transaction.NewStorage(sh, ldb)
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// seedLegacyEntry writes a complete pre-migration reserve entry: batch radius
// item, legacy 106-byte chunk bin item, chunk stamp, stamp index and payload.
func seedLegacyEntry(t *testing.T, st transaction.Storage, bin uint8, binID uint64, ch swarm.Chunk, stampHash []byte) {
	t.Helper()
	err := st.Run(context.Background(), func(s transaction.Store) error {
		return errors.Join(
			s.IndexStore().Put(&reserve.BatchRadiusItem{Bin: bin, BatchID: ch.Stamp().BatchID(), Address: ch.Address(), BinID: binID, StampHash: stampHash}),
			s.IndexStore().Put(&legacyChunkBinItem{bin: bin, binID: binID, address: ch.Address(), batchID: ch.Stamp().BatchID(), chunkType: storage.ChunkType(ch), stampHash: stampHash}),
			chunkstamp.Store(s.IndexStore(), "reserve", ch),
			stampindex.Store(s.IndexStore(), "reserve", ch),
			s.ChunkStore().Put(context.Background(), ch),
		)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestStep08ZeroStampHashRemovalFreesSlot covers a legacy entry recorded with
// an unset stamp hash. Removing it must also drop its stamp index and chunk
// stamp rows: they are keyed by batch and index, not by hash, and if they
// survive the slot is poisoned. Pullsync can then neither restore the same
// chunk nor accept a newer chunk in that slot.
func TestStep08ZeroStampHashRemovalFreesSlot(t *testing.T) {
	t.Parallel()

	st := newRealStorage(t)
	baseAddr := swarm.RandAddress(t)
	ctx := context.Background()

	batch := postagetesting.MustNewBatch().ID
	c1, err := cac.New([]byte("chunk one"))
	if err != nil {
		t.Fatal(err)
	}
	ch1 := c1.WithStamp(postagetesting.MustNewFields(batch, 5, 100))
	bin := swarm.Proximity(baseAddr.Bytes(), ch1.Address().Bytes())
	seedLegacyEntry(t, st, bin, 1, ch1, swarm.EmptyAddress.Bytes())

	if err := localmigration.Step08(st, log.Noop)(); err != nil {
		t.Fatal(err)
	}

	if _, err := stampindex.Load(st.IndexStore(), "reserve", ch1.Stamp()); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("stamp index row of the removed entry must be gone, got %v", err)
	}
	if _, err := chunkstamp.LoadWithBatchID(st.IndexStore(), "reserve", ch1.Address(), batch); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("chunk stamp row of the removed entry must be gone, got %v", err)
	}

	r, err := reserve.New(baseAddr, st, 100, kademlia.NewTopologyDriver(), log.Noop)
	if err != nil {
		t.Fatal(err)
	}
	// pullsync re-delivers the same chunk with its proper stamp
	if err := r.Put(ctx, ch1); err != nil {
		t.Fatalf("re-put of the removed chunk: %v", err)
	}
	// a mutable batch reuses the slot with a newer timestamp
	c2, err := cac.New([]byte("chunk two"))
	if err != nil {
		t.Fatal(err)
	}
	ch2 := c2.WithStamp(postagetesting.MustNewFields(batch, 5, 101))
	if err := r.Put(ctx, ch2); err != nil {
		t.Fatalf("put of a newer chunk into the freed slot: %v", err)
	}
	if got := r.Size(); got != 1 {
		t.Fatalf("reserve size %d, want 1", got)
	}
}

// TestStep08SameAddressRemovalsReleasePayload covers two entries at one
// address (a single owner chunk stamped twice) that are both removed within
// one migration page. Each removal must release one reference so the payload
// leaves the chunkstore; reading the reference count from the store while the
// batch is still pending decrements it only once and leaks the blob.
func TestStep08SameAddressRemovalsReleasePayload(t *testing.T) {
	t.Parallel()

	st := newRealStorage(t)
	baseAddr := swarm.RandAddress(t)

	// an invalid payload at a SOC address: step_08 removes such entries
	sc := soctesting.GenerateMockSOC(t, []byte("payload")).Chunk()
	bad := swarm.NewChunk(sc.Address(), []byte("garbage payload that does not validate"))
	bin := swarm.Proximity(baseAddr.Bytes(), sc.Address().Bytes())

	for i, stamp := range []swarm.Stamp{postagetesting.MustNewStamp(), postagetesting.MustNewStamp()} {
		h, err := stamp.Hash()
		if err != nil {
			t.Fatal(err)
		}
		seedLegacyEntry(t, st, bin, uint64(i+1), swarm.NewChunk(bad.Address(), bad.Data()).WithStamp(stamp), h)
	}
	rIdx := &chunkstore.RetrievalIndexItem{Address: sc.Address()}
	if err := st.IndexStore().Get(rIdx); err != nil {
		t.Fatal(err)
	}
	if rIdx.RefCnt != 2 {
		t.Fatalf("seed ref count %d, want 2", rIdx.RefCnt)
	}

	if err := localmigration.Step08(st, log.Noop)(); err != nil {
		t.Fatal(err)
	}

	if n, _ := st.IndexStore().Count(&reserve.BatchRadiusItem{}); n != 0 {
		t.Fatalf("%d reserve entries left, want 0", n)
	}
	err := st.IndexStore().Get(&chunkstore.RetrievalIndexItem{Address: sc.Address()})
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("payload with no reserve entries must leave the chunkstore, got %v", err)
	}
}
