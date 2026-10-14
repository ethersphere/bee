// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	chunk "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	kademlia "github.com/ethersphere/bee/v2/pkg/topology/mock"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// TestReserveRebuildSnapshot runs the fingerprint index rebuild protocol of the
// rangesync proposal (§5.2) against the real reserve on the storage stack the
// storer builds (transaction.Storage over an on-disk leveldbstore), while
// Reserve.Put (new chunks, same-address and different-address replacements,
// rejected stale stamps) and EvictBatchBin run concurrently.
//
// Every reserve transaction is wrapped as ReserveObserver.Commit would wrap it:
// the index read lock is held across the transaction and the delta it
// committed. A rebuild takes the write lock, opens the iterator, releases the
// lock on the first callback and then iterates without it. Commits that land
// after the iterator was opened are queued for the new slab. The test asserts
// that the snapshot plus the queued deltas equals the live index at every swap
// (every commit is in the snapshot or in the delta stream, never both) and that
// nothing deadlocks.
//
// The set element is (address, stamp hash) taken from ChunkBinItem, which is
// written and deleted in exactly the transactions that will carry the
// ChunkAddrItem of T1.4.
func TestReserveRebuildSnapshot(t *testing.T) {
	t.Parallel()

	const (
		writers         = 4
		putsPerWriter   = 250
		stampIndexes    = 80 // < putsPerWriter, so stamp indexes collide
		bins            = 3
		evictPerCall    = 8
		iterationPauses = 32 // sleep every this many items while rebuilding
	)

	baseAddr := swarm.RandAddress(t)
	ix := newModelIndex()
	st := &observedStorage{Storage: newReserveDiskStorage(t), ix: ix}

	r, err := reserve.New(baseAddr, st, 0, kademlia.NewTopologyDriver(), log.Noop)
	if err != nil {
		t.Fatal(err)
	}

	// Generate the chunks up front: GenerateTestRandomChunkAt may call t.Fatal.
	rnd := rand.New(rand.NewSource(1))
	batchIDs := make([][]byte, writers)
	work := make([][]swarm.Chunk, writers)
	for w := range writers {
		batchIDs[w] = postagetesting.MustNewBatch().ID
		for i := range putsPerWriter {
			ts := uint64(i) + 1 // increasing per batch; each writer owns one batch
			index := uint64(rnd.Intn(stampIndexes))
			var ch swarm.Chunk
			switch {
			case i > 0 && i%10 == 0:
				// same address, same stamp index, newer timestamp
				prev := work[w][rnd.Intn(i)]
				index = indexOf(prev)
				ch = swarm.NewChunk(prev.Address(), prev.Data())
			case i%15 == 0:
				// stale stamp: rejected with ErrOverwriteNewerChunk if the
				// index is taken, so the transaction fails
				ts = 0
				ch = chunk.GenerateTestRandomChunkAt(t, baseAddr, rnd.Intn(bins))
			default:
				ch = chunk.GenerateTestRandomChunkAt(t, baseAddr, rnd.Intn(bins))
			}
			work[w] = append(work[w], ch.WithStamp(postagetesting.MustNewFields(batchIDs[w], index, ts)))
		}
	}

	ctx := context.Background()
	var (
		writersWG sync.WaitGroup
		bgWG      sync.WaitGroup
		stop      = make(chan struct{})
	)

	for w := range writers {
		writersWG.Go(func() {
			for _, ch := range work[w] {
				if err := r.Put(ctx, ch); err != nil && !errors.Is(err, storage.ErrOverwriteNewerChunk) {
					t.Errorf("put: %v", err)
					return
				}
			}
		})
	}

	bgWG.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			for _, id := range batchIDs {
				if _, err := r.EvictBatchBin(ctx, id, evictPerCall, swarm.MaxBins); err != nil {
					t.Errorf("evict: %v", err)
					return
				}
			}
			time.Sleep(20 * time.Millisecond)
		}
	})

	var (
		rebuilds       int
		racedRebuilds  int // rebuilds with commits landing while the iterator was open
		nonEmptyBuilds int
	)
	bgWG.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			res, err := ix.rebuild(st, iterationPauses)
			if err != nil {
				t.Error(err)
				return
			}
			rebuilds++
			if res.pending > 0 {
				racedRebuilds++
			}
			if res.size > 0 {
				nonEmptyBuilds++
			}
			time.Sleep(time.Millisecond)
		}
	})

	done := make(chan struct{})
	go func() {
		writersWG.Wait()
		close(stop)
		bgWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Minute):
		t.Fatal("timed out: writers, eviction and rebuild deadlocked")
	}
	if t.Failed() {
		return
	}

	t.Logf("rebuilds %d, with concurrent commits %d, non-empty %d, replacements %d, failed transactions %d, final reserve size %d",
		rebuilds, racedRebuilds, nonEmptyBuilds, ix.replaced.Load(), ix.failed.Load(), r.Size())
	if racedRebuilds == 0 || nonEmptyBuilds == 0 {
		t.Fatalf("no rebuild raced with commits on a non-empty reserve (rebuilds %d, raced %d, non-empty %d)", rebuilds, racedRebuilds, nonEmptyBuilds)
	}
	if ix.replaced.Load() == 0 || ix.failed.Load() == 0 {
		t.Fatalf("replacement or failed-transaction paths not exercised (replaced %d, failed %d)", ix.replaced.Load(), ix.failed.Load())
	}

	// The deltas are exact: the live index equals the reserve.
	truth := chunkBinSet(t, st)
	if err := sameSet(ix.live, truth); err != nil {
		t.Fatalf("live index differs from the reserve: %v", err)
	}
	// A quiet rebuild equals the reserve too.
	res, err := ix.rebuild(st, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.size != len(truth) {
		t.Fatalf("rebuild size %d, reserve size %d", res.size, len(truth))
	}
}

// modelIndex stands in for fingerprint.Index: a multiset of ids, the lock
// (ix.mu) and the rebuild protocol of §5.2.
type modelIndex struct {
	mu sync.RWMutex // ix.mu: read-locked by commits, write-locked by rebuild steps 1 and 5

	dmu        sync.Mutex // guards the fields below; commits apply concurrently under mu.RLock
	live       map[string]int
	rebuilding bool
	pending    map[string]int // deltas of commits after the rebuild iterator was opened

	replaced atomic.Int64 // commits that removed and added an element
	failed   atomic.Int64 // transactions that failed, so applied nothing
}

func newModelIndex() *modelIndex {
	return &modelIndex{live: make(map[string]int)}
}

func (ix *modelIndex) apply(delta map[string]int) {
	ix.dmu.Lock()
	defer ix.dmu.Unlock()
	var adds, removes bool
	for k, v := range delta {
		adds = adds || v > 0
		removes = removes || v < 0
		ix.live[k] += v
		if ix.live[k] == 0 {
			delete(ix.live, k)
		}
		if ix.rebuilding {
			ix.pending[k] += v
		}
	}
	if adds && removes {
		ix.replaced.Add(1)
	}
}

type rebuildResult struct {
	size    int
	pending int
}

// rebuild implements §5.2 steps 1-5 and checks the new slab against the live
// one before the swap.
func (ix *modelIndex) rebuild(st transaction.Storage, pauseEvery int) (rebuildResult, error) {
	// Step 1: take the write lock and open the iterator. The storage API opens
	// the leveldb iterator inside Iterate, before the first callback, so the
	// lock is released on the first callback (or when an empty iteration
	// returns). Nothing in here calls into the reserve.
	ix.mu.Lock()
	ix.dmu.Lock()
	ix.rebuilding = true
	ix.pending = make(map[string]int)
	ix.dmu.Unlock()

	var once sync.Once
	release := func() { once.Do(ix.mu.Unlock) }

	// Step 2: build the new slab from the iterator, without the lock.
	slab := make(map[string]int)
	n := 0
	err := st.IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return &reserve.ChunkBinItem{} },
	}, func(res storage.Result) (bool, error) {
		release()
		item := res.Entry.(*reserve.ChunkBinItem)
		slab[elementID(item.Address, item.StampHash)]++
		n++
		if pauseEvery > 0 && n%pauseEvery == 0 {
			time.Sleep(time.Millisecond) // let commits land mid-iteration
		}
		return false, nil
	})
	release()
	if err != nil {
		return rebuildResult{}, err
	}

	// Steps 3-5: under the write lock no commit is in flight; apply the
	// queued deltas to the new slab and compare it with the live one.
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.dmu.Lock()
	defer ix.dmu.Unlock()

	pending := len(ix.pending)
	for k, v := range ix.pending {
		slab[k] += v
		if slab[k] == 0 {
			delete(slab, k)
		}
	}
	ix.rebuilding = false
	ix.pending = nil

	if err := sameSet(slab, ix.live); err != nil {
		return rebuildResult{}, fmt.Errorf("rebuilt slab differs from the live index: %w", err)
	}
	return rebuildResult{size: len(slab), pending: pending}, nil
}

// sameSet reports whether got is a set (every count 1) equal to want.
func sameSet(got, want map[string]int) error {
	for k, v := range got {
		if v != 1 {
			return fmt.Errorf("element %x has count %d", k, v)
		}
		if want[k] != 1 {
			return fmt.Errorf("element %x is missing on the other side", k)
		}
	}
	for k, v := range want {
		if v != 1 {
			return fmt.Errorf("element %x has count %d on the other side", k, v)
		}
	}
	if len(got) != len(want) {
		return fmt.Errorf("size %d, want %d", len(got), len(want))
	}
	return nil
}

// observedStorage wraps every Run the way ReserveObserver.Commit wraps the
// reserve's set-changing transactions in T1.4: index read lock across the
// transaction and the delta, and the delta is applied only if Run succeeded.
type observedStorage struct {
	transaction.Storage
	ix *modelIndex
}

func (o *observedStorage) Run(ctx context.Context, f func(transaction.Store) error) error {
	o.ix.mu.RLock()
	defer o.ix.mu.RUnlock()

	rec := &recordingStore{delta: make(map[string]int)}
	err := o.Storage.Run(ctx, func(s transaction.Store) error {
		rec.Store = s
		return f(rec)
	})
	if err != nil {
		o.ix.failed.Add(1)
		return err
	}
	o.ix.apply(rec.delta)
	return nil
}

// recordingStore records the exact (address, stamp hash) delta of the
// ChunkBinItem writes and deletes of one transaction.
type recordingStore struct {
	transaction.Store
	delta map[string]int
}

func (r *recordingStore) IndexStore() storage.IndexStore {
	return &recordingIndexStore{IndexStore: r.Store.IndexStore(), delta: r.delta}
}

type recordingIndexStore struct {
	storage.IndexStore
	delta map[string]int
}

func (r *recordingIndexStore) Put(i storage.Item) error {
	if item, ok := i.(*reserve.ChunkBinItem); ok {
		r.delta[elementID(item.Address, item.StampHash)]++
	}
	return r.IndexStore.Put(i)
}

func (r *recordingIndexStore) Delete(i storage.Item) error {
	if key, ok := i.(*reserve.ChunkBinItem); ok {
		// a delete counts only if the item exists (exact deltas, §5.4)
		item := &reserve.ChunkBinItem{Bin: key.Bin, BinID: key.BinID}
		switch err := r.Get(item); {
		case err == nil:
			r.delta[elementID(item.Address, item.StampHash)]--
		case !errors.Is(err, storage.ErrNotFound):
			return err
		}
	}
	return r.IndexStore.Delete(i)
}

func chunkBinSet(t *testing.T, st transaction.Storage) map[string]int {
	t.Helper()

	set := make(map[string]int)
	err := st.IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return &reserve.ChunkBinItem{} },
	}, func(res storage.Result) (bool, error) {
		item := res.Entry.(*reserve.ChunkBinItem)
		set[elementID(item.Address, item.StampHash)]++
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func elementID(addr swarm.Address, stampHash []byte) string {
	return addr.ByteString() + string(stampHash)
}

func indexOf(ch swarm.Chunk) uint64 {
	var index uint64
	for _, b := range ch.Stamp().Index() {
		index = index<<8 | uint64(b)
	}
	return index
}

type reserveDirFS struct {
	basedir string
}

func (d *reserveDirFS) Open(path string) (fs.File, error) {
	return os.OpenFile(filepath.Join(d.basedir, path), os.O_RDWR|os.O_CREATE, 0o644)
}

// newReserveDiskStorage returns transaction.Storage over an on-disk sharky and
// leveldbstore, as initDiskRepository in pkg/storer builds it, with a small
// write buffer so memtables are flushed while rebuild iterators are open.
func newReserveDiskStorage(t *testing.T) transaction.Storage {
	t.Helper()

	sharkyStore, err := sharky.New(&reserveDirFS{basedir: t.TempDir()}, 1, swarm.SocMaxChunkSize)
	if err != nil {
		t.Fatal(err)
	}
	store, _, err := leveldbstore.New(t.TempDir(), &opt.Options{
		WriteBuffer:         64 * 1024,
		CompactionL0Trigger: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	st := transaction.NewStorage(sharkyStore, store)
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Error(err)
		}
	})
	return st
}
