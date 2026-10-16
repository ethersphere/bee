// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"bytes"
	"errors"
	"math/rand/v2"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const testLeafBits = 4

var errFakeIterate = errors.New("fake iterate failed")

// fakeReserve is a reserve address index on an on-disk leveldb with a small
// write buffer, so that memtables are flushed while a rebuild iterates. Keys
// are the 64-byte IDs (address ‖ stamp hash), values the batch IDs; keys of
// other lengths are leveldbstore's own and skipped. Like the
// real reserve, every set-changing transaction runs under a per-batch lock
// and the registration read lock, wrapped by the observer's Commit with its
// exact delta; ReserveIterateAddr is one leveldb iterator (one snapshot) and
// takes no lock.
type fakeReserve struct {
	db *leveldb.DB

	regMu    sync.RWMutex
	observer storer.ReserveObserver

	locksMu sync.Mutex
	locks   map[string]*sync.Mutex

	radius atomic.Uint32
	ready  atomic.Bool

	iterCalls   atomic.Int64
	iterating   atomic.Int64 // open iterators
	overlapping atomic.Int64 // commits made while an iterator was open
	radiusReads atomic.Int64
	failNext    atomic.Int64

	hookMu       sync.Mutex
	onOpen       func()
	onElement    func(i int)
	onRadiusRead func() // called by StorageRadius after it loaded the radius
}

func newFakeReserve(t *testing.T, radius uint8) *fakeReserve {
	t.Helper()
	st, _, err := leveldbstore.New(t.TempDir(), &opt.Options{WriteBuffer: 16 << 10})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	f := &fakeReserve{db: st.DB(), locks: make(map[string]*sync.Mutex)}
	f.radius.Store(uint32(radius))
	f.ready.Store(true)
	return f
}

func (f *fakeReserve) register(o storer.ReserveObserver) {
	f.regMu.Lock()
	defer f.regMu.Unlock()
	f.observer = o
}

func (f *fakeReserve) setHooks(onOpen func(), onElement func(int)) {
	f.hookMu.Lock()
	defer f.hookMu.Unlock()
	f.onOpen, f.onElement = onOpen, onElement
}

func (f *fakeReserve) hooks() (func(), func(int)) {
	f.hookMu.Lock()
	defer f.hookMu.Unlock()
	return f.onOpen, f.onElement
}

func (f *fakeReserve) batchLock(batch []byte) *sync.Mutex {
	f.locksMu.Lock()
	defer f.locksMu.Unlock()
	l, ok := f.locks[string(batch)]
	if !ok {
		l = new(sync.Mutex)
		f.locks[string(batch)] = l
	}
	return l
}

// change runs one set-changing transaction under the batch lock and the
// registration read lock, through the observer if one is registered.
func (f *fakeReserve) change(t *testing.T, batch []byte, fn func(tx *leveldb.Batch) (adds, removes []storer.ChunkID)) {
	t.Helper()
	l := f.batchLock(batch)
	l.Lock()
	defer l.Unlock()
	f.regMu.RLock()
	defer f.regMu.RUnlock()

	run := func() ([]storer.ChunkID, []storer.ChunkID, error) {
		tx := new(leveldb.Batch)
		adds, removes := fn(tx)
		if err := f.db.Write(tx, nil); err != nil {
			return nil, nil, err
		}
		return adds, removes, nil
	}
	if f.iterating.Load() > 0 {
		f.overlapping.Add(1)
	}
	var err error
	if f.observer == nil {
		_, _, err = run()
	} else {
		err = f.observer.Commit(run)
	}
	if err != nil {
		t.Error(err)
	}
}

func (f *fakeReserve) has(t *testing.T, id fingerprint.ID) bool {
	t.Helper()
	ok, err := f.db.Has(id[:], nil)
	if err != nil {
		t.Error(err)
	}
	return ok
}

func chunkID(id fingerprint.ID, batch []byte) storer.ChunkID {
	return storer.ChunkID{Address: id.Address(), StampHash: id.StampHash(), BatchID: batch}
}

// put adds id if absent.
func (f *fakeReserve) put(t *testing.T, id fingerprint.ID, batch []byte) {
	t.Helper()
	f.change(t, batch, func(tx *leveldb.Batch) ([]storer.ChunkID, []storer.ChunkID) {
		if f.has(t, id) {
			return nil, nil
		}
		tx.Put(id[:], batch)
		return []storer.ChunkID{chunkID(id, batch)}, nil
	})
}

// evict removes id if present: the delta is exact, so evicting an element
// the address index does not hold reports nothing.
func (f *fakeReserve) evict(t *testing.T, id fingerprint.ID, batch []byte) {
	t.Helper()
	f.change(t, batch, func(tx *leveldb.Batch) ([]storer.ChunkID, []storer.ChunkID) {
		if !f.has(t, id) {
			return nil, nil
		}
		tx.Delete(id[:])
		return nil, []storer.ChunkID{chunkID(id, batch)}
	})
}

// replace removes old (if present) and adds repl (if absent) in one
// transaction.
func (f *fakeReserve) replace(t *testing.T, old, repl fingerprint.ID, batch []byte) {
	t.Helper()
	f.change(t, batch, func(tx *leveldb.Batch) (adds, removes []storer.ChunkID) {
		if f.has(t, old) {
			tx.Delete(old[:])
			removes = append(removes, chunkID(old, batch))
		}
		if !f.has(t, repl) {
			tx.Put(repl[:], batch)
			adds = append(adds, chunkID(repl, batch))
		}
		return adds, removes
	})
}

// fill writes id without reporting it, as the address index fill does.
func (f *fakeReserve) fill(t *testing.T, id fingerprint.ID, batch []byte) {
	t.Helper()
	if err := f.db.Put(id[:], batch, nil); err != nil {
		t.Fatal(err)
	}
}

// setRadius stores the radius, then notifies the observer, as
// Reserve.SetRadius does.
func (f *fakeReserve) setRadius(r uint8) {
	f.radius.Store(uint32(r))
	f.regMu.RLock()
	o := f.observer
	f.regMu.RUnlock()
	if o != nil {
		o.RadiusChanged(r)
	}
}

// reset evicts every element, one transaction each, then calls the
// observer's Reset, as Reserve.Reset does.
func (f *fakeReserve) reset(t *testing.T) {
	t.Helper()
	for id, batch := range f.contents(t) {
		f.evict(t, id, batch)
	}
	f.regMu.RLock()
	o := f.observer
	f.regMu.RUnlock()
	if o != nil {
		o.Reset()
	}
}

// contents returns the elements of the address index with their batches.
func (f *fakeReserve) contents(t *testing.T) map[fingerprint.ID][]byte {
	t.Helper()
	it := f.db.NewIterator(nil, nil)
	defer it.Release()
	out := make(map[fingerprint.ID][]byte)
	for it.Next() {
		if len(it.Key()) != fingerprint.IDSize {
			continue // leveldbstore's own keys
		}
		var id fingerprint.ID
		copy(id[:], it.Key())
		out[id] = bytes.Clone(it.Value())
	}
	if err := it.Error(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (f *fakeReserve) ReserveIterateAddr(prefix swarm.Address, bits uint8, after []byte, cb func(storer.ChunkID) (stop bool, err error)) error {
	f.iterCalls.Add(1)
	if f.failNext.Load() > 0 {
		f.failNext.Add(-1)
		return errFakeIterate
	}
	r := fingerprint.NewRange(prefix, bits)
	start := r.Prefix.Bytes()
	if after != nil {
		start = append(bytes.Clone(after), 0)
	}
	it := f.db.NewIterator(&util.Range{Start: start}, nil)
	defer it.Release()
	f.iterating.Add(1)
	defer f.iterating.Add(-1)

	onOpen, onElement := f.hooks()
	if onOpen != nil {
		onOpen()
	}
	for i := 0; it.Next(); i++ {
		k := it.Key()
		if len(k) != fingerprint.IDSize {
			continue // leveldbstore's own keys
		}
		if !r.Contains(swarm.NewAddress(k[:swarm.HashSize])) {
			break
		}
		c := storer.ChunkID{
			Address:   swarm.NewAddress(bytes.Clone(k[:swarm.HashSize])),
			StampHash: bytes.Clone(k[swarm.HashSize:]),
			BatchID:   bytes.Clone(it.Value()),
		}
		if stop, err := cb(c); err != nil || stop {
			return err
		}
		if onElement != nil {
			onElement(i)
		}
	}
	return it.Error()
}

func (f *fakeReserve) ReserveAddrIndexReady() bool { return f.ready.Load() }

func (f *fakeReserve) StorageRadius() uint8 {
	f.radiusReads.Add(1)
	r := uint8(f.radius.Load())
	f.hookMu.Lock()
	hook := f.onRadiusRead
	f.hookMu.Unlock()
	if hook != nil {
		hook()
	}
	return r
}

// fixture is an Indexer over a fake reserve.
type fixture struct {
	reserve *fakeReserve
	index   *fingerprint.Index
	indexer *rangesync.Indexer
	overlay swarm.Address
	rand    *rand.Rand
}

func newFixture(t *testing.T, radius uint8, o rangesync.IndexerOptions, logger log.Logger) *fixture {
	t.Helper()
	r := rand.New(rand.NewPCG(uint64(len(t.Name())), 7))
	overlay := make([]byte, swarm.HashSize)
	fillRand(r, overlay)
	if o.ReadyPollInterval == 0 {
		o.ReadyPollInterval = time.Millisecond
	}
	if logger == nil {
		logger = log.Noop
	}
	fx := &fixture{
		reserve: newFakeReserve(t, radius),
		index:   fingerprint.NewIndex(fingerprint.IndexOptions{LeafBits: testLeafBits}),
		overlay: swarm.NewAddress(overlay),
		rand:    r,
	}
	fx.indexer = rangesync.NewIndexer(fx.index, fx.reserve, fx.overlay, logger, o)
	fx.reserve.register(fx.indexer)
	t.Cleanup(func() {
		if err := fx.indexer.Close(); err != nil {
			t.Error(err)
		}
		if err := fx.index.Close(); err != nil {
			t.Error(err)
		}
	})
	return fx
}

func fillRand(r *rand.Rand, b []byte) {
	for i := range b {
		b[i] = byte(r.Uint32())
	}
}

// id returns a random ID whose address has proximity at least po to the
// overlay.
func (fx *fixture) id(po int) fingerprint.ID {
	var id fingerprint.ID
	fillRand(fx.rand, id[:])
	ov := fx.overlay.Bytes()
	for k := 0; k < po; k++ {
		mask := byte(0x80) >> (k % 8)
		id[k/8] = id[k/8]&^mask | ov[k/8]&mask
	}
	return id
}

// batch returns one of four batch IDs.
func (fx *fixture) batch() []byte {
	return bytes.Repeat([]byte{byte(fx.rand.IntN(4))}, swarm.HashSize)
}

func (fx *fixture) root(radius uint8) fingerprint.Range {
	return fingerprint.NewRange(fx.overlay, radius)
}

// waitRoot waits until the index has a live slab at root.
func (fx *fixture) waitRoot(t *testing.T, radius uint8) {
	t.Helper()
	root := fx.root(radius)
	deadline := time.Now().Add(10 * time.Second)
	for {
		got, _, err := fx.index.RootSummary()
		if err == nil && got.Equal(root) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("index root %s (%v), want %s", got, err, root)
		}
		time.Sleep(time.Millisecond)
	}
}

// check compares every range at every level below the radius root with
// the fake's address index.
func (fx *fixture) check(t *testing.T, radius uint8) {
	t.Helper()
	root := fx.root(radius)
	contents := fx.reserve.contents(t)
	for level := uint8(0); level <= testLeafBits; level++ {
		for _, r := range root.Children(level) {
			var (
				n uint64
				h fingerprint.LtHash
			)
			for id := range contents {
				if r.ContainsID(id) {
					n++
					h.Add(id)
				}
			}
			want := fingerprint.Summary{Count: n}
			if n > 0 {
				want.Digest = h.Digest()
			}
			got, err := fx.index.Summary(r)
			if err != nil {
				t.Fatalf("summary %s: %v", r, err)
			}
			if got != want {
				t.Fatalf("summary %s: count %d, want %d (digest equal %v)", r, got.Count, want.Count, got.Digest == want.Digest)
			}
		}
	}
}

// within runs fn and fails if it does not return within d.
func within(t *testing.T, d time.Duration, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("did not return within %v", d)
	}
}

func waitFor(tb testing.TB, what string, cond func() bool) {
	tb.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			tb.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestIndexerPutEvictReplace(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, 2, rangesync.IndexerOptions{}, nil)
	ids := make([]fingerprint.ID, 0, 100)
	for range 100 {
		id := fx.id(fx.rand.IntN(5))
		ids = append(ids, id)
		fx.reserve.put(t, id, fx.batch())
	}
	fx.indexer.Start()
	fx.waitRoot(t, 2)
	fx.check(t, 2)

	b := fx.batch()
	for i := range 100 {
		switch i % 3 {
		case 0:
			fx.reserve.put(t, fx.id(fx.rand.IntN(5)), b)
		case 1:
			fx.reserve.evict(t, ids[i], fx.contents(t, ids[i]))
		case 2:
			fx.reserve.replace(t, ids[i], fx.id(fx.rand.IntN(5)), fx.contents(t, ids[i]))
		}
	}
	// Evicting an element the address index does not hold changes nothing.
	fx.reserve.evict(t, fx.id(3), b)
	fx.check(t, 2)
}

// contents returns the batch of id in the fake, or a fresh one.
func (fx *fixture) contents(t *testing.T, id fingerprint.ID) []byte {
	t.Helper()
	if b, ok := fx.reserve.contents(t)[id]; ok {
		return b
	}
	return fx.batch()
}

// TestIndexerCommit checks that Commit runs the transaction once and
// returns its error as is.
func TestIndexerCommit(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, 0, rangesync.IndexerOptions{}, nil)
	errRun := errors.New("run failed")
	calls := 0
	err := fx.indexer.Commit(func() ([]storer.ChunkID, []storer.ChunkID, error) {
		calls++
		return nil, nil, errRun
	})
	if err != errRun || calls != 1 { //nolint:errorlint // returned as is
		t.Fatalf("commit: %v after %d calls, want the run error after 1", err, calls)
	}
}

// TestIndexerDropBeforeFirstSnapshot checks that commits made before the
// first rebuild opens its snapshot are dropped and covered by it.
func TestIndexerDropBeforeFirstSnapshot(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, 1, rangesync.IndexerOptions{}, nil)
	for range 50 {
		fx.reserve.put(t, fx.id(fx.rand.IntN(3)), fx.batch())
	}
	if _, err := fx.index.Summary(fx.root(1)); !errors.Is(err, fingerprint.ErrBusy) {
		t.Fatalf("summary before start: %v, want ErrBusy", err)
	}
	fx.indexer.Start()
	fx.waitRoot(t, 1)
	fx.check(t, 1)
}

// TestIndexerWaitsForReady checks that the first rebuild waits for the
// address index to be ready: a background fill writes elements without
// reporting them, so a snapshot opened before the fill completed would miss
// them forever.
func TestIndexerWaitsForReady(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, 1, rangesync.IndexerOptions{}, nil)
	fx.reserve.ready.Store(false)
	for range 30 {
		fx.reserve.fill(t, fx.id(fx.rand.IntN(3)), fx.batch())
		fx.reserve.put(t, fx.id(fx.rand.IntN(3)), fx.batch())
	}
	fx.indexer.Start()
	time.Sleep(20 * time.Millisecond)
	if n, m := fx.reserve.iterCalls.Load(), fx.reserve.radiusReads.Load(); n != 0 || m != 0 {
		t.Fatalf("%d iterations and %d radius reads before the address index was ready", n, m)
	}
	for range 30 {
		fx.reserve.fill(t, fx.id(fx.rand.IntN(3)), fx.batch())
	}
	fx.reserve.ready.Store(true)
	fx.waitRoot(t, 1)
	fx.check(t, 1)
}

// TestIndexerEvictedBeforeFill checks that an element evicted before the
// fill reached it (so its address index entry never existed) leaves no
// phantom difference, before and after the first rebuild.
func TestIndexerEvictedBeforeFill(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, 0, rangesync.IndexerOptions{}, nil)
	fx.reserve.ready.Store(false)
	b := fx.batch()
	for range 20 {
		fx.reserve.fill(t, fx.id(2), b)
	}
	// Chunks in the reserve that the fill has not reached yet: their
	// eviction finds no entry and reports no removal.
	for range 5 {
		fx.reserve.evict(t, fx.id(2), b)
	}
	// A chunk put (dropped: no snapshot yet) and evicted.
	put := fx.id(1)
	fx.reserve.put(t, put, b)
	fx.reserve.evict(t, put, b)

	fx.indexer.Start()
	fx.reserve.ready.Store(true)
	fx.waitRoot(t, 0)
	fx.check(t, 0)
	for range 5 {
		fx.reserve.evict(t, fx.id(2), b)
	}
	fx.check(t, 0)
}

// TestIndexerCommitsDuringFirstRebuild checks that a Put and an eviction
// that commit after the first rebuild opened its snapshot, and before the
// swap, are in the final index.
func TestIndexerCommitsDuringFirstRebuild(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, 0, rangesync.IndexerOptions{}, nil)
	b := fx.batch()
	ids := make([]fingerprint.ID, 0, 40)
	for range 40 {
		id := fx.id(0)
		ids = append(ids, id)
		fx.reserve.put(t, id, b)
	}
	reached := make(chan struct{})
	resume := make(chan struct{})
	var once sync.Once
	fx.reserve.setHooks(nil, func(i int) {
		if i == 0 {
			once.Do(func() {
				close(reached)
				<-resume
			})
		}
	})
	fx.indexer.Start()
	<-reached
	fx.reserve.put(t, fx.id(0), b)
	fx.reserve.evict(t, ids[len(ids)-1], b) // in the snapshot, not yet iterated
	fx.reserve.evict(t, ids[0], b)
	close(resume)
	fx.waitRoot(t, 0)
	fx.check(t, 0)
}

// TestIndexerRebuildUnderBatchLock opens a rebuild's snapshot while a
// commit holds a reserve batch lock and waits for the index lock: the
// iteration takes no reserve lock, so both complete.
func TestIndexerRebuildUnderBatchLock(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, 0, rangesync.IndexerOptions{}, nil)
	b := fx.batch()
	for range 20 {
		fx.reserve.put(t, fx.id(0), b)
	}
	putDone := make(chan struct{})
	var once sync.Once
	fx.reserve.setHooks(func() {
		once.Do(func() {
			// The rebuild holds the index lock in write mode: this Put takes
			// the batch lock and the registration read lock, then waits in
			// Commit.
			go func() {
				defer close(putDone)
				fx.reserve.put(t, fx.id(0), b)
			}()
			time.Sleep(20 * time.Millisecond)
		})
	}, nil)
	within(t, 10*time.Second, func() {
		fx.indexer.Start()
		fx.waitRoot(t, 0)
		<-putDone
	})
	fx.check(t, 0)
}

// TestIndexerRadiusChanged checks that RadiusChanged returns at once while a
// rebuild runs, and that the root moves.
func TestIndexerRadiusChanged(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, 2, rangesync.IndexerOptions{}, nil)
	for range 100 {
		fx.reserve.put(t, fx.id(fx.rand.IntN(4)), fx.batch())
	}
	fx.indexer.Start()
	fx.waitRoot(t, 2)

	reached := make(chan struct{})
	resume := make(chan struct{})
	var once sync.Once
	fx.reserve.setHooks(nil, func(int) {
		once.Do(func() {
			close(reached)
			<-resume
		})
	})
	within(t, time.Second, func() { fx.reserve.setRadius(1) })
	<-reached
	within(t, time.Second, func() { fx.reserve.setRadius(1) })
	// The old root keeps answering while the rebuild runs.
	if _, err := fx.index.Summary(fx.root(2)); err != nil {
		t.Fatalf("old root during rebuild: %v", err)
	}
	close(resume)
	fx.waitRoot(t, 1)
	fx.check(t, 1)

	fx.reserve.setRadius(3)
	fx.waitRoot(t, 3)
	fx.check(t, 3)
}

// TestIndexerRadiusOutOfOrder checks that stale RadiusChanged arguments do
// not matter: the index converges to the reserve's radius.
func TestIndexerRadiusOutOfOrder(t *testing.T) {
	t.Parallel()

	t.Run("5 6 5 with the 6-rebuild in flight", func(t *testing.T) {
		t.Parallel()

		fx := newFixture(t, 5, rangesync.IndexerOptions{}, nil)
		for range 60 {
			fx.reserve.put(t, fx.id(4+fx.rand.IntN(4)), fx.batch())
		}
		fx.indexer.Start()
		fx.waitRoot(t, 5)

		reached := make(chan struct{})
		resume := make(chan struct{})
		var once sync.Once
		fx.reserve.setHooks(func() {
			once.Do(func() {
				close(reached)
				<-resume
			})
		}, nil)
		fx.reserve.setRadius(6)
		<-reached
		fx.reserve.setRadius(5)
		close(resume)
		waitFor(t, "the rebuild back to 5", func() bool { return fx.reserve.iterCalls.Load() >= 3 })
		fx.waitRoot(t, 5)
		fx.check(t, 5)
	})

	t.Run("6 stored, notified 5 then 6", func(t *testing.T) {
		t.Parallel()

		fx := newFixture(t, 5, rangesync.IndexerOptions{}, nil)
		for range 60 {
			fx.reserve.put(t, fx.id(4+fx.rand.IntN(4)), fx.batch())
		}
		fx.indexer.Start()
		fx.waitRoot(t, 5)
		fx.reserve.radius.Store(6)
		fx.indexer.RadiusChanged(5)
		fx.indexer.RadiusChanged(6)
		fx.waitRoot(t, 6)
		fx.check(t, 6)
	})
}

// TestIndexerSameRadiusSkipped checks that a wake-up with the radius of the
// last completed rebuild does not rebuild.
func TestIndexerSameRadiusSkipped(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, 1, rangesync.IndexerOptions{}, nil)
	fx.reserve.put(t, fx.id(1), fx.batch())
	fx.indexer.Start()
	fx.waitRoot(t, 1)
	reads := fx.reserve.radiusReads.Load()
	fx.indexer.RadiusChanged(1)
	waitFor(t, "the radius read", func() bool { return fx.reserve.radiusReads.Load() > reads })
	time.Sleep(10 * time.Millisecond)
	if n := fx.reserve.iterCalls.Load(); n != 1 {
		t.Fatalf("%d iterations, want 1", n)
	}
	fx.check(t, 1)
}

// TestIndexerRadiusReadAfterWake checks that the rebuild goroutine takes
// the wake-up before it reads the radius: a SetRadius that lands right after
// the read must leave a wake-up behind. The hook changes the radius and
// notifies after the read returned the old value; were the wake-up taken
// after the read, the notification would coalesce into it and be lost, and
// the index would stay at the old radius.
func TestIndexerRadiusReadAfterWake(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, 1, rangesync.IndexerOptions{}, nil)
	for range 30 {
		fx.reserve.put(t, fx.id(fx.rand.IntN(3)), fx.batch())
	}
	fx.indexer.Start()
	fx.waitRoot(t, 1)

	var once sync.Once
	fx.reserve.hookMu.Lock()
	fx.reserve.onRadiusRead = func() {
		once.Do(func() { fx.reserve.setRadius(2) })
	}
	fx.reserve.hookMu.Unlock()
	fx.indexer.RadiusChanged(1)
	fx.waitRoot(t, 2)
	fx.check(t, 2)
}

// TestIndexerFailedRebuild checks that after a failed rebuild a wake-up with
// the same radius rebuilds, and that the retry runs on its own.
func TestIndexerFailedRebuild(t *testing.T) {
	t.Parallel()

	t.Run("same radius rebuilds", func(t *testing.T) {
		t.Parallel()

		fx := newFixture(t, 1, rangesync.IndexerOptions{RebuildRetryInterval: time.Hour}, nil)
		fx.reserve.put(t, fx.id(1), fx.batch())
		fx.reserve.failNext.Store(1)
		fx.indexer.Start()
		waitFor(t, "the failed rebuild", func() bool { return fx.reserve.iterCalls.Load() == 1 })
		if _, err := fx.index.Summary(fx.root(1)); !errors.Is(err, fingerprint.ErrBusy) {
			t.Fatalf("summary after a failed first rebuild: %v, want ErrBusy", err)
		}
		fx.indexer.RadiusChanged(1)
		fx.waitRoot(t, 1)
		fx.check(t, 1)

		// A failure after a completed rebuild clears the completed root too.
		fx.reserve.failNext.Store(1)
		fx.reserve.setRadius(2)
		waitFor(t, "the failed rebuild", func() bool { return fx.reserve.iterCalls.Load() == 3 })
		fx.reserve.setRadius(1)
		waitFor(t, "the rebuild at 1", func() bool { return fx.reserve.iterCalls.Load() == 4 })
		fx.waitRoot(t, 1)
		fx.check(t, 1)
	})
}

// syncBuffer is a bytes.Buffer safe for concurrent use.
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

// TestIndexerRetry checks the retry of failed rebuilds: a Warning for the
// first failure, Debug for the following ones, and a wake-up during the
// retry wait rebuilds at once.
func TestIndexerRetry(t *testing.T) {
	t.Parallel()

	t.Run("retry interval", func(t *testing.T) {
		t.Parallel()

		buf := new(syncBuffer)
		logger := log.NewLogger("test", log.WithSink(buf), log.WithVerbosity(log.VerbosityDebug))
		fx := newFixture(t, 1, rangesync.IndexerOptions{RebuildRetryInterval: 5 * time.Millisecond}, logger)
		fx.reserve.put(t, fx.id(1), fx.batch())
		fx.reserve.failNext.Store(3)
		fx.indexer.Start()
		fx.waitRoot(t, 1)
		fx.check(t, 1)
		if n := fx.reserve.iterCalls.Load(); n != 4 {
			t.Fatalf("%d iterations, want 4", n)
		}
		var warnings, debugs int
		for _, line := range strings.Split(buf.String(), "\n") {
			if !strings.Contains(line, errFakeIterate.Error()) {
				continue
			}
			switch {
			case strings.Contains(line, `"level"="warning"`):
				warnings++
			case strings.Contains(line, `"level"="debug"`):
				debugs++
			}
		}
		if warnings != 1 || debugs != 2 {
			t.Fatalf("logged %d warnings and %d debug lines, want 1 and 2:\n%s", warnings, debugs, buf.String())
		}
	})

	t.Run("wake-up during the wait", func(t *testing.T) {
		t.Parallel()

		fx := newFixture(t, 1, rangesync.IndexerOptions{RebuildRetryInterval: time.Hour}, nil)
		fx.reserve.put(t, fx.id(1), fx.batch())
		fx.reserve.failNext.Store(1)
		fx.indexer.Start()
		waitFor(t, "the failed rebuild", func() bool { return fx.reserve.iterCalls.Load() == 1 })
		fx.reserve.setRadius(2)
		fx.waitRoot(t, 2)
		fx.check(t, 2)
	})
}

// TestIndexerResetNoop checks that a reserve Reset leaves the index exact
// without a rebuild.
func TestIndexerResetNoop(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, 0, rangesync.IndexerOptions{}, nil)
	for range 50 {
		fx.reserve.put(t, fx.id(0), fx.batch())
	}
	fx.indexer.Start()
	fx.waitRoot(t, 0)
	fx.reserve.reset(t)
	time.Sleep(10 * time.Millisecond)
	if n := fx.reserve.iterCalls.Load(); n != 1 {
		t.Fatalf("%d iterations, want 1", n)
	}
	_, s, err := fx.index.RootSummary()
	if err != nil || s != (fingerprint.Summary{}) {
		t.Fatalf("root summary after reset: %+v %v, want empty", s, err)
	}
	fx.check(t, 0)
}

// TestIndexerRootIsStorageRadius checks open question 5: an address that
// IsWithinStorageRadius accepts (proximity to the overlay at least the
// radius) is exactly one inside the root NewRange(overlay, radius).
func TestIndexerRootIsStorageRadius(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(1, 5))
	overlay := make([]byte, swarm.HashSize)
	fillRand(r, overlay)
	for radius := uint8(0); radius <= swarm.MaxPO; radius++ {
		root := fingerprint.NewRange(swarm.NewAddress(overlay), radius)
		for po := 0; po <= int(swarm.MaxPO)+1; po++ {
			addr := make([]byte, swarm.HashSize)
			fillRand(r, addr)
			for k := 0; k < po && k < 8*swarm.HashSize; k++ {
				mask := byte(0x80) >> (k % 8)
				addr[k/8] = addr[k/8]&^mask | overlay[k/8]&mask
			}
			if po < 8*swarm.HashSize {
				mask := byte(0x80) >> (po % 8)
				addr[po/8] = addr[po/8]&^mask | ^overlay[po/8]&mask
			}
			within := swarm.Proximity(addr, overlay) >= radius
			if got := root.Contains(swarm.NewAddress(addr)); got != within {
				t.Fatalf("radius %d proximity %d: in root %v, within radius %v", radius, po, got, within)
			}
		}
	}
}

// TestIndexerConcurrent runs writers on several batches and Summary readers
// while the radius changes; the index ends equal to the reserve. Flushes of
// the small leveldb write buffer happen during the iterations. Run it with
// -race.
func TestIndexerConcurrent(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, 2, rangesync.IndexerOptions{}, nil)
	for range 200 {
		fx.reserve.put(t, fx.id(fx.rand.IntN(4)), fx.batch())
	}
	fx.indexer.Start()
	fx.waitRoot(t, 2)
	// Slow the rebuilds down so that commits overlap them.
	fx.reserve.setHooks(nil, func(i int) {
		if i%16 == 0 {
			time.Sleep(50 * time.Microsecond)
		}
	})

	// Writers run until the radius loop below has seen enough overlap.
	const minOverlap = 20
	stopWriters := make(chan struct{})
	var writers sync.WaitGroup
	for w := range 4 {
		writers.Add(1)
		go func() {
			defer writers.Done()
			r := rand.New(rand.NewPCG(uint64(w), 3))
			b := bytes.Repeat([]byte{byte(10 + w)}, swarm.HashSize)
			var mine []fingerprint.ID
			newID := func() fingerprint.ID {
				var id fingerprint.ID
				fillRand(r, id[:])
				copy(id[:1], fx.overlay.Bytes()[:1]) // proximity >= 8 most of the time
				return id
			}
			for i := 0; ; i++ {
				select {
				case <-stopWriters:
					return
				default:
				}
				switch {
				case len(mine) > 100 || len(mine) > 0 && i%4 == 1:
					k := r.IntN(len(mine))
					fx.reserve.evict(t, mine[k], b)
					mine = append(mine[:k], mine[k+1:]...)
				case len(mine) > 0 && i%4 == 2:
					k := r.IntN(len(mine))
					id := newID()
					fx.reserve.replace(t, mine[k], id, b)
					mine[k] = id
				default:
					id := newID()
					fx.reserve.put(t, id, b)
					mine = append(mine, id)
				}
			}
		}()
	}
	stop := make(chan struct{})
	var readers sync.WaitGroup
	for range 2 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				for level := uint8(0); level < 3; level++ {
					_, _ = fx.index.Summary(fx.root(level))
				}
			}
		}()
	}
	deadline := time.Now().Add(20 * time.Second)
	for i := 0; i < 6 || fx.reserve.overlapping.Load() < minOverlap; i++ {
		if time.Now().After(deadline) {
			break // reported below, after the goroutines stopped
		}
		fx.reserve.setRadius(uint8(i % 3))
		time.Sleep(2 * time.Millisecond)
	}
	close(stopWriters)
	writers.Wait()
	close(stop)
	readers.Wait()
	if n := fx.reserve.overlapping.Load(); n < minOverlap {
		t.Fatalf("%d commits overlapped a rebuild iteration, want at least %d", n, minOverlap)
	}

	// Every radius read after setRadius(1) returned reads 1, and one
	// follows (its wake-up); after it the goroutine skips or rebuilds at 1.
	fx.reserve.setRadius(1)
	reads := fx.reserve.radiusReads.Load()
	waitFor(t, "the last radius read", func() bool { return fx.reserve.radiusReads.Load() > reads })
	fx.waitRoot(t, 1)
	fx.check(t, 1)

	if v, err := fx.reserve.db.GetProperty("leveldb.num-files-at-level0"); err != nil || v == "0" {
		t.Logf("no memtable flush (level-0 files %q, %v)", v, err)
	}
}

// TestIndexerClose checks that Close cancels a running rebuild, keeps the
// previous slab, and stops the goroutine (goleak in TestMain).
func TestIndexerClose(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, 1, rangesync.IndexerOptions{}, nil)
	for range 300 {
		fx.reserve.put(t, fx.id(0), fx.batch())
	}
	fx.indexer.Start()
	fx.waitRoot(t, 1)

	reached := make(chan struct{})
	var once sync.Once
	fx.reserve.setHooks(nil, func(int) {
		once.Do(func() { close(reached) })
		time.Sleep(time.Millisecond)
	})
	fx.reserve.setRadius(0)
	<-reached
	within(t, 2*time.Second, func() {
		if err := fx.indexer.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := fx.indexer.Close(); err != nil {
		t.Fatal(err)
	}
	fx.waitRoot(t, 1)
	fx.check(t, 1)

	// An Indexer that never started closes too.
	idle := rangesync.NewIndexer(fingerprint.NewIndex(fingerprint.IndexOptions{}), fx.reserve, fx.overlay, log.Noop, rangesync.IndexerOptions{})
	within(t, time.Second, func() { _ = idle.Close() })
}
