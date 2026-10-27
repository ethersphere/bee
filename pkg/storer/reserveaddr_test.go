// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage"
	chunk "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type addrIndexOpts struct {
	enabled  bool
	keep     bool
	capacity int
}

func addrIndexTestOps(baseAddr swarm.Address, o addrIndexOpts) *storer.Options {
	capacity := o.capacity
	if capacity == 0 {
		capacity = 1000
	}
	if capacity < 0 {
		capacity = 0
	}
	opts := dbTestOps(baseAddr, capacity, nil, nil, time.Hour)
	opts.ReserveAddrIndex = o.enabled
	opts.ReserveAddrIndexKeep = o.keep
	return opts
}

// openAddrIndexDB opens a storer the test closes itself (closeDB).
func openAddrIndexDB(t *testing.T, dir string, baseAddr swarm.Address, o addrIndexOpts) *storer.DB {
	t.Helper()

	db, err := storer.New(context.Background(), dir, addrIndexTestOps(baseAddr, o))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return db
}

func closeDB(t *testing.T, db *storer.DB) {
	t.Helper()

	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// putReserveChunks puts n stamped chunks of the batch into the reserve.
func putReserveChunks(t *testing.T, db *storer.DB, baseAddr swarm.Address, batchID []byte, n int) []swarm.Chunk {
	t.Helper()

	chunks := make([]swarm.Chunk, 0, n)
	for i := range n {
		ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, i%4).WithStamp(postagetesting.MustNewBatchStamp(batchID))
		if err := db.ReservePutter().Put(context.Background(), ch); err != nil {
			t.Fatal(err)
		}
		chunks = append(chunks, ch)
	}
	return chunks
}

func elementKey(addr swarm.Address, stampHash []byte) string {
	return addr.ByteString() + string(stampHash)
}

// addrIndexSet returns the address index entries in the store.
func addrIndexSet(t *testing.T, db *storer.DB) map[string]string {
	t.Helper()

	set := make(map[string]string)
	err := db.Storage().IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return &reserve.ChunkAddrItem{} },
	}, func(res storage.Result) (bool, error) {
		set[res.ID] = string(res.Entry.(*reserve.ChunkAddrItem).BatchID)
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return set
}

// reserveSet returns the (address, stamp hash) set of the reserve.
func reserveSet(t *testing.T, db *storer.DB) map[string]string {
	t.Helper()

	set := make(map[string]string)
	err := db.Storage().IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return &reserve.ChunkBinItem{} },
	}, func(res storage.Result) (bool, error) {
		item := res.Entry.(*reserve.ChunkBinItem)
		set[elementKey(item.Address, item.StampHash)] = string(item.BatchID)
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func checkIndexEqualsReserve(t *testing.T, db *storer.DB) {
	t.Helper()

	got, want := addrIndexSet(t, db), reserveSet(t, db)
	if len(want) == 0 {
		t.Fatal("empty reserve: the check covers nothing")
	}
	if len(got) != len(want) {
		t.Fatalf("address index has %d elements, reserve %d", len(got), len(want))
	}
	for k, b := range want {
		if got[k] != b {
			t.Fatalf("element %x: index batch %x, reserve batch %x", k, got[k], b)
		}
	}
}

func marker(t *testing.T, db *storer.DB) (*reserve.ChunkAddrValidItem, bool) {
	t.Helper()

	m := &reserve.ChunkAddrValidItem{}
	err := db.Storage().IndexStore().Get(m)
	switch {
	case errors.Is(err, storage.ErrNotFound):
		return nil, false
	case err != nil:
		t.Fatal(err)
	}
	return m, true
}

func digest(t *testing.T, db *storer.DB) reserve.AddrIndexDigest {
	t.Helper()

	d, err := reserve.ReadAddrIndexDigest(context.Background(), db.Storage().IndexStore())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func checkAddrStoreDisabled(t *testing.T, db *storer.DB) {
	t.Helper()

	ctx := context.Background()
	if db.ReserveAddrIndexReady() {
		t.Fatal("disabled address index reports ready")
	}
	err := db.ReserveIterateAddr(swarm.ZeroAddress, 0, nil, func(storer.ChunkID) (bool, error) { return false, nil })
	if !errors.Is(err, storer.ErrAddrIndexDisabled) {
		t.Fatalf("ReserveIterateAddr: %v", err)
	}
	if _, err := db.ReserveGetByID(ctx, swarm.RandAddress(t), make([]byte, 32)); !errors.Is(err, storer.ErrAddrIndexDisabled) {
		t.Fatalf("ReserveGetByID: %v", err)
	}
	if _, err := db.ReserveHasID(swarm.RandAddress(t), make([]byte, 32)); !errors.Is(err, storer.ErrAddrIndexDisabled) {
		t.Fatalf("ReserveHasID: %v", err)
	}
	if err := db.SetReserveObserver(&countingObserver{}); !errors.Is(err, storer.ErrAddrIndexDisabled) {
		t.Fatalf("SetReserveObserver: %v", err)
	}
}

type countingObserver struct {
	adds, removes, resets atomic.Int32
}

func (o *countingObserver) Commit(run func() (adds, removes []storer.ChunkID, err error)) error {
	adds, removes, err := run()
	o.adds.Add(int32(len(adds)))
	o.removes.Add(int32(len(removes)))
	return err
}
func (o *countingObserver) RadiusChanged(uint8) {}
func (o *countingObserver) Reset()              { o.resets.Add(1) }

// TestReserveAddrIndexDefaultNoWrites checks that with the defaults nothing of
// the address index is ever written, through Put, eviction and reset.
func TestReserveAddrIndexDefaultNoWrites(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseAddr := swarm.RandAddress(t)
	dir := t.TempDir()
	db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{})
	batch := postagetesting.MustNewBatch()
	putReserveChunks(t, db, baseAddr, batch.ID, 10)
	if _, err := db.Reserve().EvictBatchBin(ctx, batch.ID, 5, swarm.MaxBins); err != nil {
		t.Fatal(err)
	}
	checkAddrStoreDisabled(t, db)
	if err := db.ResetReserve(ctx); err != nil {
		t.Fatal(err)
	}
	closeDB(t, db)

	// and again after a reopen with the defaults
	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{})
	defer closeDB(t, db)
	putReserveChunks(t, db, baseAddr, batch.ID, 3)
	if n := len(addrIndexSet(t, db)); n != 0 {
		t.Fatalf("%d address index entries written with the defaults", n)
	}
	if _, ok := marker(t, db); ok {
		t.Fatal("marker written with the defaults")
	}
	if db.AddrIndexFills() != 0 {
		t.Fatal("fill ran with the defaults")
	}
}

// TestReserveAddrIndexToggle turns the index on, off and on again with
// reserve changes in between: each enabled start trusts or refills the index
// correctly and it always equals the reserve.
func TestReserveAddrIndexToggle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseAddr := swarm.RandAddress(t)
	dir := t.TempDir()
	b1, b2, b3 := postagetesting.MustNewBatch(), postagetesting.MustNewBatch(), postagetesting.MustNewBatch()

	// on: an empty reserve is filled (trivially) and marked
	db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
	if db.AddrIndexFills() != 1 || !db.ReserveAddrIndexReady() {
		t.Fatalf("first enabled start: fills %d ready %v", db.AddrIndexFills(), db.ReserveAddrIndexReady())
	}
	putReserveChunks(t, db, baseAddr, b1.ID, 10)
	putReserveChunks(t, db, baseAddr, b2.ID, 10)
	checkIndexEqualsReserve(t, db)
	closeDB(t, db)

	// on again, unchanged: no refill
	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
	if db.AddrIndexFills() != 0 || !db.ReserveAddrIndexReady() {
		t.Fatalf("clean reopen: fills %d ready %v", db.AddrIndexFills(), db.ReserveAddrIndexReady())
	}
	if _, err := db.Reserve().EvictBatchBin(ctx, b1.ID, 3, swarm.MaxBins); err != nil {
		t.Fatal(err)
	}
	checkIndexEqualsReserve(t, db)
	closeDB(t, db)

	// off: the marker goes at once, the entries in the background, and the
	// index is no longer written
	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{})
	if _, ok := marker(t, db); ok {
		t.Fatal("marker kept with the index disabled")
	}
	checkAddrStoreDisabled(t, db)
	db.WaitAddrIndexDrop()
	if n := len(addrIndexSet(t, db)); n != 0 {
		t.Fatalf("%d entries left after the drop", n)
	}
	putReserveChunks(t, db, baseAddr, b3.ID, 5)
	if _, err := db.Reserve().EvictBatchBin(ctx, b2.ID, 4, swarm.MaxBins); err != nil {
		t.Fatal(err)
	}
	if n := len(addrIndexSet(t, db)); n != 0 {
		t.Fatalf("%d entries written with the index disabled", n)
	}
	closeDB(t, db)

	// on: refill, with the changes made while off and no stale element
	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
	defer closeDB(t, db)
	if db.AddrIndexFills() != 1 || !db.ReserveAddrIndexReady() {
		t.Fatalf("enabled start after off: fills %d ready %v", db.AddrIndexFills(), db.ReserveAddrIndexReady())
	}
	checkIndexEqualsReserve(t, db)
}

// TestReserveAddrIndexDropInterrupted checks that a drop interrupted by Close
// resumes at the next disabled start.
func TestReserveAddrIndexDropInterrupted(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)
	dir := t.TempDir()
	db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
	putReserveChunks(t, db, baseAddr, postagetesting.MustNewBatch().ID, 20)
	closeDB(t, db)

	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{})
	closeDB(t, db) // may stop the drop at any point

	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{})
	defer closeDB(t, db)
	db.WaitAddrIndexDrop()
	if n := len(addrIndexSet(t, db)); n != 0 {
		t.Fatalf("%d entries left", n)
	}
}

// syncBuffer is a bytes.Buffer that a logger may write to concurrently.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestReserveAddrIndexReclaimLog (T6.5 part 2): once the background drop of
// a disabled address index and the compaction of its key range are done, the
// storer logs it at Info, with the entries dropped and the time it took, so
// that an operator sees when the space is back.
func TestReserveAddrIndexReclaimLog(t *testing.T) {
	t.Parallel()

	const chunks = 20
	baseAddr := swarm.RandAddress(t)
	dir := t.TempDir()
	db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
	putReserveChunks(t, db, baseAddr, postagetesting.MustNewBatch().ID, chunks)
	closeDB(t, db)

	buf := new(syncBuffer)
	opts := addrIndexTestOps(baseAddr, addrIndexOpts{})
	opts.Logger = log.NewLogger("test", log.WithSink(buf), log.WithVerbosity(log.VerbosityInfo))
	db, err := storer.New(context.Background(), dir, opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer closeDB(t, db)
	if !db.AddrIndexDropStarted() {
		t.Fatal("no background drop of the disabled index")
	}
	db.WaitAddrIndexDrop()

	var lines []string
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.Contains(line, `"msg"="reclaimed the disabled reserve address index"`) {
			lines = append(lines, line)
		}
	}
	if len(lines) != 1 {
		t.Fatalf("%d reclaim lines, want 1:\n%s", len(lines), buf.String())
	}
	for _, want := range []string{`"level"="info"`, fmt.Sprintf(`"count"=%d`, chunks), `"duration"=`} {
		if !strings.Contains(lines[0], want) {
			t.Fatalf("reclaim line %q lacks %s", lines[0], want)
		}
	}
}

func TestReserveAddrIndexMarkerVersion(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)
	dir := t.TempDir()
	db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
	putReserveChunks(t, db, baseAddr, postagetesting.MustNewBatch().ID, 8)
	closeDB(t, db)

	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{keep: true})
	m, ok := marker(t, db)
	if !ok {
		t.Fatal("no marker after a clean close")
	}
	m.Version++
	if err := db.Storage().Run(context.Background(), func(s transaction.Store) error { return s.IndexStore().Put(m) }); err != nil {
		t.Fatal(err)
	}
	closeDB(t, db)

	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
	defer closeDB(t, db)
	if db.AddrIndexFills() != 1 {
		t.Fatal("a marker of another version was trusted")
	}
	checkIndexEqualsReserve(t, db)
}

// copyDir copies a directory tree, as a crash would leave it.
func copyDir(t *testing.T, src, dst string) {
	t.Helper()

	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, in)
		return errors.Join(err, out.Close())
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestReserveAddrIndexUncleanShutdown checks that a start after an unclean
// shutdown (a copy of the store taken while it was open) refills the index,
// while a start after a clean close does not.
func TestReserveAddrIndexUncleanShutdown(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)
	dir := t.TempDir()
	crashed := t.TempDir()
	db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
	putReserveChunks(t, db, baseAddr, postagetesting.MustNewBatch().ID, 12)
	copyDir(t, dir, crashed)
	closeDB(t, db)

	db = openAddrIndexDB(t, crashed, baseAddr, addrIndexOpts{enabled: true})
	if db.AddrIndexFills() != 1 {
		t.Fatal("no refill after an unclean shutdown")
	}
	checkIndexEqualsReserve(t, db)
	closeDB(t, db)

	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
	defer closeDB(t, db)
	if db.AddrIndexFills() != 0 {
		t.Fatal("refill after a clean close")
	}
	checkIndexEqualsReserve(t, db)
}

// TestReserveAddrIndexKeepMode checks that keep mode leaves the marker and the
// entries alone, and that a reserve change in keep mode is still caught.
func TestReserveAddrIndexKeepMode(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)
	dir := t.TempDir()
	db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
	putReserveChunks(t, db, baseAddr, postagetesting.MustNewBatch().ID, 8)
	closeDB(t, db)

	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{keep: true})
	before, ok := marker(t, db)
	if !ok {
		t.Fatal("keep mode deleted the marker")
	}
	checkAddrStoreDisabled(t, db)
	closeDB(t, db)

	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{keep: true})
	after, ok := marker(t, db)
	if !ok || *after != *before {
		t.Fatal("keep mode changed the marker")
	}
	closeDB(t, db)

	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
	if db.AddrIndexFills() != 0 {
		t.Fatal("refill after keep mode without changes")
	}
	closeDB(t, db)

	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{keep: true})
	putReserveChunks(t, db, baseAddr, postagetesting.MustNewBatch().ID, 3)
	closeDB(t, db)

	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
	defer closeDB(t, db)
	if db.AddrIndexFills() != 1 {
		t.Fatal("a reserve change in keep mode was not caught")
	}
	checkIndexEqualsReserve(t, db)
}

// TestReserveAddrIndexNoReserve checks that a storer without a reserve treats
// the index as disabled.
func TestReserveAddrIndexNoReserve(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)
	dir := t.TempDir()
	db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
	putReserveChunks(t, db, baseAddr, postagetesting.MustNewBatch().ID, 8)
	closeDB(t, db)

	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true, capacity: -1})
	defer closeDB(t, db)
	if _, ok := marker(t, db); ok {
		t.Fatal("marker kept without a reserve")
	}
	db.WaitAddrIndexDrop()
	if n := len(addrIndexSet(t, db)); n != 0 {
		t.Fatalf("%d entries left without a reserve", n)
	}
	checkAddrStoreDisabled(t, db)
}

func TestReserveAddrStoreDisabled(t *testing.T) {
	t.Parallel()

	for _, o := range []addrIndexOpts{{}, {keep: true}, {enabled: true, capacity: -1}} {
		t.Run(fmt.Sprintf("%+v", o), func(t *testing.T) {
			t.Parallel()
			db := openAddrIndexDB(t, "", swarm.RandAddress(t), o)
			defer closeDB(t, db)
			checkAddrStoreDisabled(t, db)
		})
	}
}

func TestSetReserveObserver(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)
	db := openAddrIndexDB(t, "", baseAddr, addrIndexOpts{enabled: true})
	defer closeDB(t, db)

	if err := db.SetReserveObserver(nil); !errors.Is(err, storer.ErrNilObserver) {
		t.Fatalf("nil observer: %v", err)
	}
	o := &countingObserver{}
	if err := db.SetReserveObserver(o); err != nil {
		t.Fatal(err)
	}
	if err := db.SetReserveObserver(&countingObserver{}); !errors.Is(err, storer.ErrObserverRegistered) {
		t.Fatalf("second observer: %v", err)
	}
	putReserveChunks(t, db, baseAddr, postagetesting.MustNewBatch().ID, 3)
	if o.adds.Load() != 3 {
		t.Fatalf("observer saw %d adds, want 3", o.adds.Load())
	}
	if err := db.ResetReserve(context.Background()); err != nil {
		t.Fatal(err)
	}
	if o.removes.Load() != 3 || o.resets.Load() != 1 {
		t.Fatalf("reset: %d removes, %d resets", o.removes.Load(), o.resets.Load())
	}
}

// TestReserveAddrStore checks the DB methods against the reserve content.
func TestReserveAddrStore(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseAddr := swarm.RandAddress(t)
	db := openAddrIndexDB(t, "", baseAddr, addrIndexOpts{enabled: true})
	defer closeDB(t, db)
	if !db.ReserveAddrIndexReady() {
		t.Fatal("not ready after New")
	}

	chunks := putReserveChunks(t, db, baseAddr, postagetesting.MustNewBatch().ID, 30)
	want := make([]string, 0, len(chunks))
	byKey := make(map[string]swarm.Chunk)
	for _, ch := range chunks {
		sh, err := ch.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}
		k := elementKey(ch.Address(), sh)
		want = append(want, k)
		byKey[k] = ch
	}
	slices.Sort(want)

	var got []string
	err := db.ReserveIterateAddr(swarm.ZeroAddress, 0, nil, func(c storer.ChunkID) (bool, error) {
		got = append(got, elementKey(c.Address, c.StampHash))
		if !bytes.Equal(c.BatchID, byKey[elementKey(c.Address, c.StampHash)].Stamp().BatchID()) {
			return true, errors.New("wrong batch ID")
		}
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("iterated %d elements, want %d in order", len(got), len(want))
	}

	// resume after the tenth element, within the range of its first byte
	after := []byte(want[9])
	got = got[:0]
	err = db.ReserveIterateAddr(swarm.NewAddress(after[:1]), 8, after, func(c storer.ChunkID) (bool, error) {
		got = append(got, elementKey(c.Address, c.StampHash))
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var wantAfter []string
	for _, k := range want[10:] {
		if k[0] == after[0] {
			wantAfter = append(wantAfter, k)
		}
	}
	if !slices.Equal(got, wantAfter) {
		t.Fatalf("resumed iteration: %d elements, want %d", len(got), len(wantAfter))
	}

	for _, ch := range chunks[:5] {
		sh, _ := ch.Stamp().Hash()
		got, err := db.ReserveGetByID(ctx, ch.Address(), sh)
		if err != nil || !got.Equal(ch) {
			t.Fatalf("ReserveGetByID: %v", err)
		}
		if has, err := db.ReserveHasID(ch.Address(), sh); err != nil || !has {
			t.Fatalf("ReserveHasID: %v, %v", has, err)
		}
	}
	if _, err := db.ReserveGetByID(ctx, swarm.RandAddress(t), make([]byte, 32)); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("ReserveGetByID of an unknown element: %v", err)
	}
	if has, err := db.ReserveHasID(swarm.RandAddress(t), make([]byte, 32)); err != nil || has {
		t.Fatalf("ReserveHasID of an unknown element: %v, %v", has, err)
	}
}

// TestReserveAddrIndexClose checks that a clean Close rewrites the marker with
// the digest of the reserve as it is then.
func TestReserveAddrIndexClose(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)
	dir := t.TempDir()
	db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
	putReserveChunks(t, db, baseAddr, postagetesting.MustNewBatch().ID, 6)
	if m, _ := marker(t, db); m.Digest == digest(t, db) {
		t.Fatal("marker matches before Close; the test covers nothing")
	}
	closeDB(t, db)

	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{keep: true})
	defer closeDB(t, db)
	if m, ok := marker(t, db); !ok || m.Digest != digest(t, db) {
		t.Fatal("Close did not rewrite the marker")
	}
}

// TestReserveAddrIndexReset checks that a reset keeps the index ready and
// costs one refill at the next start (the epoch changes).
func TestReserveAddrIndexReset(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseAddr := swarm.RandAddress(t)
	dir := t.TempDir()
	db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
	putReserveChunks(t, db, baseAddr, postagetesting.MustNewBatch().ID, 6)
	if err := db.ResetReserve(ctx); err != nil {
		t.Fatal(err)
	}
	if !db.ReserveAddrIndexReady() {
		t.Fatal("not ready after a reset")
	}
	if n := len(addrIndexSet(t, db)); n != 0 {
		t.Fatalf("%d entries after a reset", n)
	}
	if m, ok := marker(t, db); !ok || m.Digest.Epoch != 0 || m.Digest.Count != 0 {
		t.Fatalf("marker after a reset %+v", m)
	}
	closeDB(t, db)

	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
	defer closeDB(t, db)
	if db.AddrIndexFills() != 1 {
		t.Fatal("no refill after a reset")
	}
	putReserveChunks(t, db, baseAddr, postagetesting.MustNewBatch().ID, 2)
	checkIndexEqualsReserve(t, db)
}

// TestRecoveryDeletesChunkAddrItem checks that start-up recovery deletes the
// address index entry of a corrupted chunk when a marker is present, also
// with the index not enabled (here: keep mode, which leaves the rest alone).
func TestRecoveryDeletesChunkAddrItem(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)
	dir := t.TempDir()
	db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
	chunks := putReserveChunks(t, db, baseAddr, postagetesting.MustNewBatch().ID, 4)
	bad := chunks[0]

	var badLoc sharky.Location
	err := db.Storage().IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return new(chunkstore.RetrievalIndexItem) },
	}, func(r storage.Result) (bool, error) {
		item := r.Entry.(*chunkstore.RetrievalIndexItem)
		if item.Address.Equal(bad.Address()) {
			badLoc = item.Location
		}
		return false, nil
	})
	if err != nil || badLoc.Length == 0 {
		t.Fatalf("locate the chunk: %v", err)
	}
	closeDB(t, db)

	// unclean shutdown with the chunk's data destroyed
	if err := os.WriteFile(filepath.Join(dir, "sharky", ".DIRTY"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "sharky", fmt.Sprintf("shard_%03d", badLoc.Shard)), os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt(make([]byte, badLoc.Length), int64(badLoc.Slot)*int64(swarm.SocMaxChunkSize)); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{keep: true})
	defer closeDB(t, db)
	set := addrIndexSet(t, db)
	badHash, _ := bad.Stamp().Hash()
	if _, ok := set[elementKey(bad.Address(), badHash)]; ok {
		t.Fatal("recovery kept the corrupted chunk's address index entry")
	}
	if len(set) != len(chunks)-1 {
		t.Fatalf("%d entries left, want %d", len(set), len(chunks)-1)
	}
}

// TestReserveAddrIndexFillCanceled checks that a fill interrupted in New (its
// context canceled, as by a shutdown) leaves a partial index and no marker,
// and that the next start refills the index exactly.
func TestReserveAddrIndexFillCanceled(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)
	dir := t.TempDir()

	// a reserve with neither an index nor a marker
	db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{})
	putReserveChunks(t, db, baseAddr, postagetesting.MustNewBatch().ID, 30)
	closeDB(t, db)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := addrIndexTestOps(baseAddr, addrIndexOpts{enabled: true})
	storer.SetReserveTestOptions(opts,
		reserve.WithAddrIndexBatchSize(3),
		reserve.WithAddrIndexFillHook(func(filled int) {
			if filled >= 6 {
				cancel()
			}
		}),
	)
	if db, err := storer.New(ctx, dir, opts); !errors.Is(err, context.Canceled) {
		if err == nil {
			closeDB(t, db)
		}
		t.Fatalf("New with an interrupted fill: %v", err)
	}

	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{keep: true})
	if _, ok := marker(t, db); ok {
		t.Fatal("marker written by an interrupted fill")
	}
	partial, total := len(addrIndexSet(t, db)), len(reserveSet(t, db))
	if partial == 0 || partial >= total {
		t.Fatalf("interrupted fill wrote %d of %d entries; the test does not cover a partial index", partial, total)
	}
	closeDB(t, db)

	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
	defer closeDB(t, db)
	if db.AddrIndexFills() != 1 || !db.ReserveAddrIndexReady() {
		t.Fatalf("start after an interrupted fill: fills %d ready %v", db.AddrIndexFills(), db.ReserveAddrIndexReady())
	}
	checkIndexEqualsReserve(t, db)
}

// TestReserveAddrIndexCompaction checks that the storer compacts the address
// index key range after a drop, both the background drop of a disabled index
// and the synchronous drop of a stale one: the dropped entries leave the
// leveldb tables instead of staying there under tombstones.
func TestReserveAddrIndexCompaction(t *testing.T) {
	t.Parallel()

	baseAddr := swarm.RandAddress(t)
	dir := t.TempDir()
	db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{enabled: true})
	putReserveChunks(t, db, baseAddr, postagetesting.MustNewBatch().ID, 200)
	closeDB(t, db)

	// Reopening moves the journal into the tables. Deleting the marker
	// makes the index stale for the enabled start.
	db = openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{keep: true})
	before, err := db.AddrIndexRangeSize()
	if err != nil {
		t.Fatal(err)
	}
	if before == 0 {
		t.Fatal("no address index entries in the tables; the test covers nothing")
	}
	m, ok := marker(t, db)
	if !ok {
		t.Fatal("no marker after a clean close")
	}
	if err := db.Storage().Run(context.Background(), func(s transaction.Store) error { return s.IndexStore().Delete(m) }); err != nil {
		t.Fatal(err)
	}
	closeDB(t, db)
	stale := t.TempDir()
	copyDir(t, dir, stale)

	rangeSize := func(t *testing.T, db *storer.DB) int64 {
		t.Helper()
		n, err := db.AddrIndexRangeSize()
		if err != nil {
			t.Fatal(err)
		}
		return n
	}

	t.Run("disabled", func(t *testing.T) {
		t.Parallel()

		db := openAddrIndexDB(t, dir, baseAddr, addrIndexOpts{})
		defer closeDB(t, db)
		db.WaitAddrIndexDrop()
		if n := len(addrIndexSet(t, db)); n != 0 {
			t.Fatalf("%d entries left after the drop", n)
		}
		if after := rangeSize(t, db); after != 0 {
			t.Fatalf("address index range holds %d bytes in the tables after the drop (%d before)", after, before)
		}
	})

	t.Run("stale", func(t *testing.T) {
		t.Parallel()

		db := openAddrIndexDB(t, stale, baseAddr, addrIndexOpts{enabled: true})
		defer closeDB(t, db)
		if db.AddrIndexFills() != 1 {
			t.Fatal("no refill of a stale index")
		}
		checkIndexEqualsReserve(t, db)
		// The refill writes to the memtable, which the size does not count.
		if after := rangeSize(t, db); after != 0 {
			t.Fatalf("address index range holds %d bytes in the tables after the drop and refill (%d before)", after, before)
		}
	})
}
