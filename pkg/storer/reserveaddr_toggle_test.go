// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	batchstore "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/storage"
	chunk "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	localmigration "github.com/ethersphere/bee/v2/pkg/storer/migration"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// The tests of this file are the storer half of plan task T6.2 (toggle
// safety, upgrade and downgrade); the node half is in pkg/node.

// element is one (chunk address, stamp hash) pair of the reserve, the unit
// the address index and rangesync work with.
type element struct {
	addr      swarm.Address
	stampHash []byte
}

func (e element) key() string { return elementKey(e.addr, e.stampHash) }

func elementOfKey(k string) element {
	return element{addr: swarm.NewAddress([]byte(k[:swarm.HashSize])), stampHash: []byte(k[swarm.HashSize:])}
}

// staleElements returns the elements of before that the reserve no longer
// holds.
func staleElements(t *testing.T, db *storer.DB, before map[string]string) []element {
	t.Helper()

	now := reserveSet(t, db)
	var stale []element
	for k := range before {
		if _, ok := now[k]; !ok {
			stale = append(stale, elementOfKey(k))
		}
	}
	return stale
}

// iterateAddrSet returns what ReserveIterateAddr yields for the range of the
// first bits bits (at most 8) of prefix: element keys and batch IDs.
func iterateAddrSet(t *testing.T, db *storer.DB, prefix swarm.Address, bits uint8) map[string]string {
	t.Helper()

	set := make(map[string]string)
	err := db.ReserveIterateAddr(prefix, bits, nil, func(c storer.ChunkID) (bool, error) {
		if bits > 0 && c.Address.Bytes()[0]>>(8-bits) != prefix.Bytes()[0]>>(8-bits) {
			return true, fmt.Errorf("element %s outside the range %x/%d", c.Address, prefix.Bytes(), bits)
		}
		k := elementKey(c.Address, c.StampHash)
		if _, ok := set[k]; ok {
			return true, fmt.Errorf("element %s iterated twice", c.Address)
		}
		set[k] = string(c.BatchID)
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return set
}

// checkAddrStoreServes checks what the ReserveAddrStore methods serve against
// the reserve (its ChunkBinItems, which the address index does not maintain):
// ReserveIterateAddr over the whole address space and over every range of 1
// to 3 bits yields exactly the reserve's elements with their batch IDs,
// ReserveHasID and ReserveGetByID find each of them, and no stale element (one
// the reserve no longer holds) is iterated, reported or returned.
func checkAddrStoreServes(t *testing.T, db *storer.DB, stale []element) {
	t.Helper()

	ctx := context.Background()
	want := reserveSet(t, db)
	if len(want) == 0 {
		t.Fatal("empty reserve: the check covers nothing")
	}
	if got := iterateAddrSet(t, db, swarm.ZeroAddress, 0); !maps.Equal(got, want) {
		t.Fatalf("ReserveIterateAddr serves %d elements, the reserve holds %d", len(got), len(want))
	}
	for bits := uint8(1); bits <= 3; bits++ {
		union := make(map[string]string)
		for p := range 1 << bits {
			maps.Copy(union, iterateAddrSet(t, db, swarm.NewAddress([]byte{byte(p << (8 - bits))}), bits))
		}
		if !maps.Equal(union, want) {
			t.Fatalf("ReserveIterateAddr serves %d elements over the ranges of %d bits, the reserve holds %d", len(union), bits, len(want))
		}
	}
	for k, batchID := range want {
		e := elementOfKey(k)
		if has, err := db.ReserveHasID(e.addr, e.stampHash); err != nil || !has {
			t.Fatalf("ReserveHasID of element %s of the reserve: %v, %v", e.addr, has, err)
		}
		ch, err := db.ReserveGetByID(ctx, e.addr, e.stampHash)
		if err != nil {
			t.Fatalf("ReserveGetByID of element %s of the reserve: %v", e.addr, err)
		}
		sh, err := ch.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}
		if !ch.Address().Equal(e.addr) || !bytes.Equal(sh, e.stampHash) || !bytes.Equal(ch.Stamp().BatchID(), []byte(batchID)) {
			t.Fatalf("ReserveGetByID of element %s returned another element", e.addr)
		}
	}
	for _, e := range stale {
		if _, ok := want[e.key()]; ok {
			t.Fatalf("stale element %s is in the reserve: the check covers nothing", e.addr)
		}
		if has, err := db.ReserveHasID(e.addr, e.stampHash); err != nil || has {
			t.Fatalf("ReserveHasID of stale element %s: %v, %v", e.addr, has, err)
		}
		if _, err := db.ReserveGetByID(ctx, e.addr, e.stampHash); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("ReserveGetByID of stale element %s: %v", e.addr, err)
		}
	}
}

// newerStamp returns a stamp of st's batch and stamp index with a newer
// timestamp.
func newerStamp(st swarm.Stamp) swarm.Stamp {
	return postagetesting.MustNewFields(st.BatchID(), binary.BigEndian.Uint64(st.Index()), binary.BigEndian.Uint64(st.Timestamp())+1)
}

// restamped returns ch with a newer stamp: the reserve replaces the element
// (same address, newer stamp).
func restamped(ch swarm.Chunk) swarm.Chunk {
	return swarm.NewChunk(ch.Address(), ch.Data()).WithStamp(newerStamp(ch.Stamp()))
}

// overwriting returns a new chunk that takes the stamp index of ch over: the
// reserve removes ch (another address, same stamp index).
func overwriting(t *testing.T, baseAddr swarm.Address, ch swarm.Chunk) swarm.Chunk {
	t.Helper()

	return chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(newerStamp(ch.Stamp()))
}

// putChunks puts the chunks into the reserve.
func putChunks(t *testing.T, db *storer.DB, chunks ...swarm.Chunk) {
	t.Helper()

	for _, ch := range chunks {
		if err := db.ReservePutter().Put(context.Background(), ch); err != nil {
			t.Fatal(err)
		}
	}
}

// crashDuringFill leaves in crashed what an enabled start of the store in dir
// leaves when it is killed while filling the address index (the files of the
// open store, copied between two fill transactions), and returns how many
// entries the fill had committed then. The store in dir must hold neither a
// marker nor index entries; its own start completes.
func crashDuringFill(t *testing.T, dir, crashed string, baseAddr swarm.Address) int {
	t.Helper()

	opts := addrIndexTestOps(baseAddr, addrIndexOpts{enabled: true})
	committed := 0
	storer.SetReserveTestOptions(opts,
		reserve.WithAddrIndexBatchSize(3),
		reserve.WithAddrIndexFillHook(func(filled int) {
			if committed == 0 && filled >= 9 {
				committed = filled
				copyDir(t, dir, crashed)
			}
		}),
	)
	db, err := storer.New(context.Background(), dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	closeDB(t, db)
	if committed == 0 {
		t.Fatal("the fill ended before the crash")
	}
	return committed
}

// repairReserve runs what `bee db repair-reserve` runs: the storer opened
// with the defaults, then the reserve repairer.
func repairReserve(t *testing.T, dir string, baseAddr swarm.Address, damage func(db *storer.DB)) {
	t.Helper()

	db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{})
	if damage != nil {
		damage(db)
	}
	if err := localmigration.ReserveRepairer(db.Storage(), storage.ChunkType, log.Noop)(); err != nil {
		t.Fatalf("repair: %v", err)
	}
	if _, ok := marker(t, db); ok {
		t.Fatal("marker kept by the repair")
	}
	if n := len(addrIndexSet(t, db)); n != 0 {
		t.Fatalf("%d address index entries kept by the repair", n)
	}
	closeDB(t, db)
}

// deleteChunkData deletes the chunk data of the reserve chunks, as a lost
// sharky shard would; the reserve entries stay.
func deleteChunkData(t *testing.T, db *storer.DB, chunks ...swarm.Chunk) {
	t.Helper()

	ctx := context.Background()
	err := db.Storage().Run(ctx, func(s transaction.Store) error {
		var err error
		for _, ch := range chunks {
			err = errors.Join(err, s.ChunkStore().Delete(ctx, ch.Address()))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestReserveAddrIndexToggleServed turns the index on, off and on again, with
// reserve changes while it is off that remove elements in every way the
// reserve does (eviction, a newer stamp for the same address, a stamp index
// taken over by another chunk) and add new ones. While off, nothing is
// served; after the refill exactly the reserve is served and no element
// removed while off is iterated, reported or returned. The disabled start's
// background drop either completes, or is interrupted before it deleted any
// entry (the worst case; the entries of the enabled period are written back
// after the disabled period, without the marker, which the disabled start
// deleted first).
func TestReserveAddrIndexToggleServed(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name            string
		dropInterrupted bool
	}{
		{name: "drop completed"},
		{name: "drop interrupted", dropInterrupted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			baseAddr := swarm.RandAddress(t)
			dir := t.TempDir()
			b1, b2, b3 := postagetesting.MustNewBatch(), postagetesting.MustNewBatch(), postagetesting.MustNewBatch()

			db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
			putReserveChunks(t, db, baseAddr, b1.ID, 12)
			c2 := putReserveChunks(t, db, baseAddr, b2.ID, 12)
			checkAddrStoreServes(t, db, nil)
			before := reserveSet(t, db)
			leftovers := rawNamespace(t, db, "chunkAddr")
			closeDB(t, db)

			db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{})
			checkAddrStoreDisabled(t, db)
			db.WaitAddrIndexDrop()
			putReserveChunks(t, db, baseAddr, b3.ID, 8)
			if _, err := db.Reserve().EvictBatchBin(ctx, b1.ID, 12, swarm.MaxBins); err != nil {
				t.Fatal(err)
			}
			putChunks(t, db, restamped(c2[0]), restamped(c2[1]), overwriting(t, baseAddr, c2[2]))
			checkAddrStoreDisabled(t, db)
			if n := len(addrIndexSet(t, db)); n != 0 {
				t.Fatalf("%d entries with the index disabled", n)
			}
			closeDB(t, db)

			if tc.dropInterrupted {
				db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{keep: true})
				putRaw(t, db, "chunkAddr", leftovers)
				if _, ok := marker(t, db); ok {
					t.Fatal("marker kept by the disabled start")
				}
				closeDB(t, db)
			}

			db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
			defer closeDB(t, db)
			if db.AddrIndexFills() != 1 || !db.ReserveAddrIndexReady() {
				t.Fatalf("enabled start after off: fills %d ready %v", db.AddrIndexFills(), db.ReserveAddrIndexReady())
			}
			stale := staleElements(t, db, before)
			if len(stale) != 12+3 {
				t.Fatalf("%d elements removed while off, want 15", len(stale))
			}
			checkIndexEqualsReserve(t, db)
			checkAddrStoreServes(t, db, stale)
		})
	}
}

// TestReserveAddrIndexRepair runs `bee db repair-reserve` (the storer opened
// with the defaults, then the repairer, which drops the index and its marker)
// on a store with an enabled index, one of whose chunks lost its data and is
// removed by the repair. The next enabled start refills the index, which
// serves exactly the repaired reserve. The damaged chunk is alone in its bin:
// see TestReserveRepairMissingChunk for what the repairer does otherwise.
func TestReserveAddrIndexRepair(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)
	dir := t.TempDir()
	batch := postagetesting.MustNewBatch()
	db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
	putReserveChunks(t, db, baseAddr, batch.ID, 20)
	lone := chunk.GenerateTestRandomChunkAt(t, baseAddr, 6).WithStamp(postagetesting.MustNewBatchStamp(batch.ID))
	putChunks(t, db, lone)
	before := reserveSet(t, db)
	closeDB(t, db)

	repairReserve(t, dir, baseAddr, func(db *storer.DB) { deleteChunkData(t, db, lone) })

	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
	defer closeDB(t, db)
	if db.AddrIndexFills() != 1 || !db.ReserveAddrIndexReady() {
		t.Fatalf("enabled start after the repair: fills %d ready %v", db.AddrIndexFills(), db.ReserveAddrIndexReady())
	}
	stale := staleElements(t, db, before)
	if len(stale) != 1 || !stale[0].addr.Equal(lone.Address()) {
		t.Fatalf("the repair removed %d elements, want the damaged one", len(stale))
	}
	checkIndexEqualsReserve(t, db)
	checkAddrStoreServes(t, db, stale)
}

// TestReserveRepairMissingChunk is a T6.2 finding about the repairer, not the
// address index (the code predates it): ReserveRepairer deletes every
// ChunkBinItem (its step 2), then renumbers the bins from 1 in parallel (step
// 3), where it removes a chunk whose data is missing with
// RemoveChunkWithItem, which deletes the ChunkBinItem of the chunk's old bin
// ID. If another chunk of the bin was given that bin ID first, that chunk's
// new ChunkBinItem is deleted instead: the repair fails its own final check
// (and without it would leave a reserve chunk no bin iteration finds). Here
// the missing chunk had bin ID 1 and is the last one the repairer handles.
func TestReserveRepairMissingChunk(t *testing.T) {
	t.Parallel()
	t.Skip("T6.2 finding, see the phase-6 Follow-ups: ReserveRepairer deletes a renumbered chunk's ChunkBinItem when another chunk of its bin is missing")

	baseAddr := swarm.RandAddress(t)
	batch := postagetesting.MustNewBatch()
	chunks := make([]swarm.Chunk, 4*runtime.NumCPU()+4)
	for i := range chunks {
		chunks[i] = chunk.GenerateTestRandomChunkAt(t, baseAddr, 0).WithStamp(postagetesting.MustNewBatchStamp(batch.ID))
	}
	// The repairer handles one bin of one batch in address order. The
	// highest address is put first (bin ID 1) and handled last.
	slices.SortFunc(chunks, func(a, b swarm.Chunk) int { return bytes.Compare(b.Address().Bytes(), a.Address().Bytes()) })
	db := openAddrIndexDB(t, t.TempDir(), baseAddr, addrIndexOpts{capacity: 2 * len(chunks)})
	defer closeDB(t, db)
	putChunks(t, db, chunks...)
	deleteChunkData(t, db, chunks[0])
	if err := localmigration.ReserveRepairer(db.Storage(), storage.ChunkType, log.Noop)(); err != nil {
		t.Fatalf("repair: %v", err)
	}
}

// TestReserveAddrIndexCrashDuringFill kills an enabled start in the middle of
// its fill (the store's files copied while it is open, between two fill
// transactions; unlike TestReserveAddrIndexFillCanceled the store is not
// closed): the copy holds part of the index and no marker, and its next
// enabled start refills the index.
func TestReserveAddrIndexCrashDuringFill(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)
	dir, crashed := t.TempDir(), t.TempDir()
	db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{})
	putReserveChunks(t, db, baseAddr, postagetesting.MustNewBatch().ID, 30)
	closeDB(t, db)

	committed := crashDuringFill(t, dir, crashed, baseAddr)

	db = openAddrIndexDB(t, crashed, baseAddr, addrIndexOpts{keep: true})
	if _, ok := marker(t, db); ok {
		t.Fatal("marker written before the fill completed")
	}
	partial, total := len(addrIndexSet(t, db)), len(reserveSet(t, db))
	if partial < committed || partial >= total {
		t.Fatalf("the crash left %d of %d entries (%d committed); the test does not cover a partial index", partial, total, committed)
	}
	closeDB(t, db)

	db = openAddrIndexDB(t, crashed, baseAddr, addrIndexOpts{enabled: true})
	defer closeDB(t, db)
	if db.AddrIndexFills() != 1 || !db.ReserveAddrIndexReady() {
		t.Fatalf("start after the crash: fills %d ready %v", db.AddrIndexFills(), db.ReserveAddrIndexReady())
	}
	checkIndexEqualsReserve(t, db)
	checkAddrStoreServes(t, db, nil)
}

// dirSize returns the total size of the files under dir.
func dirSize(t *testing.T, dir string) int64 {
	t.Helper()

	var size int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		size += info.Size()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return size
}

// restart opens the store in keep mode and closes it, so that leveldb moves
// its journal into its tables, as at any restart.
func restart(t *testing.T, dir string, baseAddr swarm.Address) {
	t.Helper()

	closeDB(t, openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{keep: true}))
}

// TestReserveAddrIndexDiskUsage measures the leveldb directory of a sharky
// and leveldb storer holding the same reserve, each time after a restart:
// never indexed, with the address index filled, and after a start with the
// index disabled (the background drop, then the compaction of the index key
// range). The disabled start gives the space the index took back.
func TestReserveAddrIndexDiskUsage(t *testing.T) {
	t.Parallel()

	const n = 4000
	baseAddr := swarm.RandAddress(t)
	batch := postagetesting.MustNewBatch()
	chunks := make([]swarm.Chunk, n)
	for i := range chunks {
		chunks[i] = chunk.GenerateTestRandomChunkAt(t, baseAddr, i%2).WithStamp(postagetesting.MustNewBatchStamp(batch.ID))
	}
	ldbSize := func(dir string) int64 { return dirSize(t, filepath.Join(dir, "indexstore")) }

	plain := t.TempDir()
	db := openAddrIndexDB(t, plain, baseAddr, addrIndexOpts{capacity: 2 * n})
	putChunks(t, db, chunks...)
	closeDB(t, db)
	restart(t, plain, baseAddr)
	sizePlain := ldbSize(plain)

	dir := t.TempDir()
	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true, capacity: 2 * n})
	putChunks(t, db, chunks...)
	checkIndexEqualsReserve(t, db)
	closeDB(t, db)
	restart(t, dir, baseAddr)
	sizeOn := ldbSize(dir)

	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{capacity: 2 * n})
	db.WaitAddrIndexDrop() // the drop, then the compaction
	if got := len(addrIndexSet(t, db)); got != 0 {
		t.Fatalf("%d entries left after the drop", got)
	}
	if rs, err := db.AddrIndexRangeSize(); err != nil || rs != 0 {
		t.Fatalf("the address index range holds %d bytes after the compaction: %v", rs, err)
	}
	closeDB(t, db)
	restart(t, dir, baseAddr)
	sizeOff := ldbSize(dir)

	index := sizeOn - sizePlain
	t.Logf("leveldb directory, %d chunks: never indexed %d B; indexed %d B (%+d B, %.1f B per chunk); after the disabled start %d B (%+d B against never indexed)",
		n, sizePlain, sizeOn, index, float64(index)/n, sizeOff, sizeOff-sizePlain)
	if index < 50*n {
		t.Fatalf("the filled index takes %d B for %d chunks: the measurement covers nothing", index, n)
	}
	if sizeOff > sizePlain+index/10 {
		t.Fatalf("disk usage did not go back down: never indexed %d B, indexed %d B, after the disabled start %d B", sizePlain, sizeOn, sizeOff)
	}
}

// TestReserveAddrIndexDefaultsAfterToggle starts a store with the defaults
// after each of the toggle cases: the first such start deletes the marker and
// drops the leftover entries in the background; from then on nothing of the
// index is written (puts, evictions) and later starts with the defaults find
// nothing to drop.
func TestReserveAddrIndexDefaultsAfterToggle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	enabled := func(t *testing.T, dir string, baseAddr swarm.Address) {
		t.Helper()
		db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
		putReserveChunks(t, db, baseAddr, postagetesting.MustNewBatch().ID, 10)
		closeDB(t, db)
	}

	for _, tc := range []struct {
		name string
		// leftovers: the case leaves index entries
		leftovers bool
		prepare   func(t *testing.T, dir string, baseAddr swarm.Address)
	}{
		{name: "on", leftovers: true, prepare: enabled},
		{name: "on, off", prepare: func(t *testing.T, dir string, baseAddr swarm.Address) {
			t.Helper()
			enabled(t, dir, baseAddr)
			db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{})
			db.WaitAddrIndexDrop()
			closeDB(t, db)
		}},
		{name: "on, reset", prepare: func(t *testing.T, dir string, baseAddr swarm.Address) {
			t.Helper()
			enabled(t, dir, baseAddr)
			db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
			if err := db.ResetReserve(ctx); err != nil {
				t.Fatal(err)
			}
			putReserveChunks(t, db, baseAddr, postagetesting.MustNewBatch().ID, 4)
			closeDB(t, db)
		}, leftovers: true},
		{name: "on, keep mode change", leftovers: true, prepare: func(t *testing.T, dir string, baseAddr swarm.Address) {
			t.Helper()
			enabled(t, dir, baseAddr)
			db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{keep: true})
			putReserveChunks(t, db, baseAddr, postagetesting.MustNewBatch().ID, 3)
			closeDB(t, db)
		}},
		{name: "on, repair", prepare: func(t *testing.T, dir string, baseAddr swarm.Address) {
			t.Helper()
			enabled(t, dir, baseAddr)
			repairReserve(t, dir, baseAddr, nil)
		}},
		{name: "crash during fill", leftovers: true, prepare: func(t *testing.T, dir string, baseAddr swarm.Address) {
			t.Helper()
			src := t.TempDir()
			db := openAddrIndexDB(t, src, baseAddr, addrIndexOpts{})
			putReserveChunks(t, db, baseAddr, postagetesting.MustNewBatch().ID, 30)
			closeDB(t, db)
			crashDuringFill(t, src, dir, baseAddr)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			baseAddr := swarm.RandAddress(t)
			dir := t.TempDir()
			tc.prepare(t, dir, baseAddr)

			db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{})
			if _, ok := marker(t, db); ok {
				t.Fatal("marker kept with the defaults")
			}
			if db.AddrIndexDropStarted() != tc.leftovers {
				t.Fatalf("drop started %v, want %v", db.AddrIndexDropStarted(), tc.leftovers)
			}
			checkAddrStoreDisabled(t, db)
			db.WaitAddrIndexDrop()
			batch := postagetesting.MustNewBatch()
			putReserveChunks(t, db, baseAddr, batch.ID, 6)
			if _, err := db.Reserve().EvictBatchBin(ctx, batch.ID, 3, swarm.MaxBins); err != nil {
				t.Fatal(err)
			}
			if n := len(addrIndexSet(t, db)); n != 0 {
				t.Fatalf("%d address index entries with the defaults", n)
			}
			if db.AddrIndexFills() != 0 {
				t.Fatal("fill ran with the defaults")
			}
			closeDB(t, db)

			db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{})
			defer closeDB(t, db)
			if db.AddrIndexDropStarted() {
				t.Fatal("a later start with the defaults started a drop")
			}
			putReserveChunks(t, db, baseAddr, batch.ID, 2)
			if _, ok := marker(t, db); ok {
				t.Fatal("marker written with the defaults")
			}
			if n := len(addrIndexSet(t, db)); n != 0 {
				t.Fatalf("%d address index entries with the defaults", n)
			}
			checkAddrStoreDisabled(t, db)
		})
	}
}

// rawItem is an entry of any namespace with an opaque value, to write and
// read entries regardless of their item type.
type rawItem struct {
	ns, id string
	val    []byte
}

func (r *rawItem) ID() string               { return r.id }
func (r *rawItem) Namespace() string        { return r.ns }
func (r *rawItem) Marshal() ([]byte, error) { return r.val, nil }
func (r *rawItem) Unmarshal(b []byte) error { r.val = slices.Clone(b); return nil }
func (r *rawItem) Clone() storage.Item      { return &rawItem{ns: r.ns, id: r.id, val: slices.Clone(r.val)} }
func (r *rawItem) String() string           { return r.ns + "/" + r.id }

// rawNamespace returns the entries of the namespace, ID to value.
func rawNamespace(t *testing.T, db *storer.DB, ns string) map[string]string {
	t.Helper()

	set := make(map[string]string)
	err := db.Storage().IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return &rawItem{ns: ns} },
	}, func(res storage.Result) (bool, error) {
		set[res.ID] = string(res.Entry.(*rawItem).val)
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return set
}

// putRaw writes the entries (ID to value) into the namespace.
func putRaw(t *testing.T, db *storer.DB, ns string, entries map[string]string) {
	t.Helper()

	err := db.Storage().Run(context.Background(), func(s transaction.Store) error {
		for id, val := range entries {
			if err := s.IndexStore().Put(&rawItem{ns: ns, id: id, val: []byte(val)}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// readReserve runs, against the reserve, what a node does with it besides
// rangesync: the reserve worker's count within the radius (radius 0 here),
// the reserve iteration of the db commands and of pullsync's bin
// subscriptions, the last bin IDs, the neighborhood statistics, the db info,
// and a reserve sample. Each must see exactly the reserve.
func readReserve(t *testing.T, db *storer.DB, baseAddr swarm.Address) {
	t.Helper()

	ctx := context.Background()
	want := reserveSet(t, db)
	n, err := db.CountWithinRadius(ctx)
	if err != nil || n != len(want) {
		t.Fatalf("count within radius %d, want %d: %v", n, len(want), err)
	}

	iterated := 0
	if err := db.ReserveIterateChunks(func(swarm.Chunk) (bool, error) { iterated++; return false, nil }); err != nil || iterated != len(want) {
		t.Fatalf("reserve iteration: %d chunks, want %d: %v", iterated, len(want), err)
	}

	perBin := make(map[uint8]int)
	for k := range want {
		perBin[swarm.Proximity(baseAddr.Bytes(), []byte(k[:swarm.HashSize]))]++
	}
	for bin, count := range perBin {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		c, stop, errC := db.SubscribeBin(ctx, bin, 0)
		for range count {
			select {
			case bc := <-c:
				if _, ok := want[elementKey(bc.Address, bc.StampHash)]; !ok {
					t.Fatalf("bin %d subscription: element %s not in the reserve", bin, bc.Address)
				}
			case err := <-errC:
				t.Fatalf("bin %d subscription: %v", bin, err)
			case <-ctx.Done():
				t.Fatalf("bin %d subscription: %v", bin, ctx.Err())
			}
		}
		stop()
		cancel()
	}

	if _, _, err := db.ReserveLastBinIDs(); err != nil {
		t.Fatal(err)
	}
	stats, err := db.NeighborhoodsStat(ctx)
	if err != nil {
		t.Fatal(err)
	}
	inNeighborhoods := 0
	for _, s := range stats {
		inNeighborhoods += s.ReserveSizeWithinRadius
	}
	if inNeighborhoods != len(want) {
		t.Fatalf("neighborhood stats count %d chunks, want %d", inNeighborhoods, len(want))
	}

	info, err := db.DebugInfo(ctx)
	if err != nil || info.Reserve.TotalSize != len(want) {
		t.Fatalf("db info reserve size %d, want %d: %v", info.Reserve.TotalSize, len(want), err)
	}

	sample, err := db.ReserveSample(ctx, swarm.RandAddress(t).Bytes(), 0, uint64(time.Now().UnixNano()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if s := sample.Stats; s.TotalIterated != int64(len(want)) || s.ChunkLoadFailed != 0 || s.StampLoadFailed != 0 {
		t.Fatalf("reserve sample stats %+v, want %d chunks iterated and loaded", s, len(want))
	}
}

// TestReserveAddrIndexDowngrade runs a store with leftover address index
// entries and marker of an enabled start through a build without rangesync,
// then upgrades it.
//
// The older build is simulated by keep mode, which skips the index's whole
// start-up step (no read, no write) and opens the reserve without the index,
// as a build without the index does; only the ReserveAddrStore methods (which
// such a build lacks) and start-up recovery's one marker read (a Has, outside
// the index namespace) differ. Entries whose values do not decode are added
// to the leftovers, so that any read of the namespace would fail. The older
// build starts, counts, iterates, samples and changes the reserve (puts,
// evictions) and starts again, with no error and with the leftovers
// untouched. The upgraded build's enabled start refills the index (the marker
// no longer matches the reserve) and serves exactly the reserve; its disabled
// start reclaims the space.
func TestReserveAddrIndexDowngrade(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseAddr := swarm.RandAddress(t)
	dir := t.TempDir()
	b1, b2, b3 := postagetesting.MustNewBatch(), postagetesting.MustNewBatch(), postagetesting.MustNewBatch()

	db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
	putReserveChunks(t, db, baseAddr, b1.ID, 16)
	putReserveChunks(t, db, baseAddr, b2.ID, 16)
	closeDB(t, db)

	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{keep: true})
	garbage := make(map[string]string)
	for range 8 {
		garbage[string(append(swarm.RandAddress(t).Bytes(), swarm.RandAddress(t).Bytes()...))] = "\xff"
	}
	putRaw(t, db, "chunkAddr", garbage)
	leftovers, leftoverMarker := rawNamespace(t, db, "chunkAddr"), rawNamespace(t, db, "addrIndexValid")
	if len(leftovers) != 32+8 || len(leftoverMarker) != 1 {
		t.Fatalf("leftovers: %d entries, %d markers", len(leftovers), len(leftoverMarker))
	}
	closeDB(t, db)

	// the older build
	opts := addrIndexTestOps(baseAddr, addrIndexOpts{keep: true})
	opts.Batchstore = batchstore.New(batchstore.WithAcceptAllExistsFunc())
	opts.ValidStamp = func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }
	db, err := storer.New(ctx, dir, opts)
	if err != nil {
		t.Fatalf("start of the older build with leftovers: %v", err)
	}
	before := reserveSet(t, db)
	readReserve(t, db, baseAddr)
	putReserveChunks(t, db, baseAddr, b3.ID, 8)
	if _, err := db.Reserve().EvictBatchBin(ctx, b2.ID, 16, swarm.MaxBins); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Reserve().EvictBatchBin(ctx, b1.ID, 4, swarm.MaxBins); err != nil {
		t.Fatal(err)
	}
	readReserve(t, db, baseAddr)
	if got := rawNamespace(t, db, "chunkAddr"); !maps.Equal(got, leftovers) {
		t.Fatalf("the older build changed the leftover entries: %d, had %d", len(got), len(leftovers))
	}
	if got := rawNamespace(t, db, "addrIndexValid"); !maps.Equal(got, leftoverMarker) {
		t.Fatal("the older build changed the leftover marker")
	}
	stale := staleElements(t, db, before)
	if len(stale) != 16+4 {
		t.Fatalf("%d elements removed by the older build, want 20", len(stale))
	}
	closeDB(t, db)

	db, err = storer.New(ctx, dir, opts)
	if err != nil {
		t.Fatalf("restart of the older build with leftovers: %v", err)
	}
	readReserve(t, db, baseAddr)
	if got := rawNamespace(t, db, "chunkAddr"); !maps.Equal(got, leftovers) {
		t.Fatal("the older build changed the leftover entries at its restart")
	}
	closeDB(t, db)
	reclaim := t.TempDir()
	copyDir(t, dir, reclaim)

	t.Run("upgrade", func(t *testing.T) {
		t.Parallel()

		db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
		defer closeDB(t, db)
		if db.AddrIndexFills() != 1 || !db.ReserveAddrIndexReady() {
			t.Fatalf("enabled start after the downgrade: fills %d ready %v", db.AddrIndexFills(), db.ReserveAddrIndexReady())
		}
		checkIndexEqualsReserve(t, db)
		checkAddrStoreServes(t, db, stale)
	})

	t.Run("reclaim", func(t *testing.T) {
		t.Parallel()

		db := openAddrIndexDB(t, reclaim, baseAddr, addrIndexOpts{})
		defer closeDB(t, db)
		db.WaitAddrIndexDrop()
		if n := len(rawNamespace(t, db, "chunkAddr")); n != 0 {
			t.Fatalf("%d entries left after the disabled start", n)
		}
		if n := len(rawNamespace(t, db, "addrIndexValid")); n != 0 {
			t.Fatal("marker left after the disabled start")
		}
	})
}
