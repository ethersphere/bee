// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build contention

// Temporary debugging harness: measures reserve.Put latency under concurrent
// load on a real on-disk leveldb + sharky store, and writes block/mutex
// profiles. Run with:
//
//	go test -tags=contention -run TestPutContention -v ./pkg/storer/internal/reserve/
package reserve_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/log"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	kademlia "github.com/ethersphere/bee/v2/pkg/topology/mock"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"resenje.org/multex"
)

type dirFS struct{ basedir string }

func (d *dirFS) Open(path string) (fs.File, error) {
	return os.OpenFile(filepath.Join(d.basedir, path), os.O_RDWR|os.O_CREATE, 0o600)
}

type env struct {
	r    *reserve.Reserve
	ldb  *leveldbstore.Store
	base swarm.Address
}

// newDiskReserve mirrors pkg/storer/storer.go initDiskRepository options.
func newDiskReserve(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sharky"), 0o700); err != nil {
		t.Fatal(err)
	}
	sh, err := sharky.New(&dirFS{basedir: filepath.Join(dir, "sharky")}, 32, swarm.SocMaxChunkSize)
	if err != nil {
		t.Fatal(err)
	}
	ldb, _, err := leveldbstore.New(filepath.Join(dir, "indexstore"), &opt.Options{
		OpenFilesCacheCapacity: 256,
		BlockCacheCapacity:     32 * 1024 * 1024,
		WriteBuffer:            32 * 1024 * 1024,
		CompactionL0Trigger:    8,
		Filter:                 filter.NewBloomFilter(64),
	})
	if err != nil {
		t.Fatal(err)
	}
	st := transaction.NewStorage(sh, ldb)
	t.Cleanup(func() { _ = st.Close() })

	base := swarm.RandAddress(t)
	r, err := reserve.New(base, st, 0, kademlia.NewTopologyDriver(), log.Noop)
	if err != nil {
		t.Fatal(err)
	}
	return &env{r: r, ldb: ldb, base: base}
}

// chunkAt brute-forces a valid CAC (full 4096-byte payload) whose address has
// proximity order po to base.
func chunkAt(base swarm.Address, po int, batch []byte, idx uint64) (swarm.Chunk, error) {
	data := make([]byte, swarm.ChunkSize)
	for {
		_, _ = rand.Read(data)
		ch, err := cac.New(data)
		if err != nil {
			return nil, err
		}
		if int(swarm.Proximity(base.Bytes(), ch.Address().Bytes())) == po {
			return ch.WithStamp(postagetesting.MustNewFields(batch, idx, uint64(time.Now().UnixNano()))), nil
		}
	}
}

func mustChunkAt(t *testing.T, base swarm.Address, po int, batch []byte, idx uint64) swarm.Chunk {
	t.Helper()
	ch, err := chunkAt(base, po, batch, idx)
	if err != nil {
		t.Fatal(err)
	}
	return ch
}

type stats struct {
	n                   int
	wall                time.Duration
	p50, p99, p999, max time.Duration
	throughput          float64
	ldbDelayN           int32
	ldbDelay            time.Duration
}

func (s stats) String() string {
	return fmt.Sprintf("n=%d wall=%v puts/s=%.0f p50=%v p99=%v p99.9=%v max=%v ldb_write_delay_n=%d ldb_write_delay=%v",
		s.n, s.wall.Round(time.Millisecond), s.throughput, s.p50, s.p99, s.p999, s.max, s.ldbDelayN, s.ldbDelay)
}

func summarize(lat []time.Duration, wall time.Duration, e *env) stats {
	slices.Sort(lat)
	q := func(p float64) time.Duration { return lat[int(math.Min(float64(len(lat)-1), p*float64(len(lat))))] }
	var ls leveldb.DBStats
	_ = e.ldb.DB().Stats(&ls)
	return stats{
		n: len(lat), wall: wall, throughput: float64(len(lat)) / wall.Seconds(),
		p50: q(0.5), p99: q(0.99), p999: q(0.999), max: lat[len(lat)-1],
		ldbDelayN: ls.WriteDelayCount, ldbDelay: ls.WriteDelayDuration,
	}
}

// runPuts puts the chunks from `workers` goroutines and returns the per-Put latency.
func runPuts(t *testing.T, e *env, chunks [][]swarm.Chunk) ([]time.Duration, time.Duration) {
	t.Helper()
	var (
		mu  sync.Mutex
		all []time.Duration
		wg  sync.WaitGroup
	)
	start := time.Now()
	for _, cs := range chunks {
		wg.Go(func() {
			lat := make([]time.Duration, 0, len(cs))
			for _, ch := range cs {
				s := time.Now()
				if err := e.r.Put(context.Background(), ch); err != nil {
					t.Error(err)
					return
				}
				lat = append(lat, time.Since(s))
			}
			mu.Lock()
			all = append(all, lat...)
			mu.Unlock()
		})
	}
	wg.Wait()
	return all, time.Since(start)
}

// gen builds workers*perWorker chunks; bins(w, i) picks the PO and each
// worker gets its own batch (as different uploaders would).
func gen(t *testing.T, e *env, workers, perWorker int, bins func(w, i int) int) [][]swarm.Chunk {
	t.Helper()
	out := make([][]swarm.Chunk, workers)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			batch := make([]byte, 32)
			_, _ = rand.Read(batch)
			for i := range perWorker {
				ch, err := chunkAt(e.base, bins(w, i), batch, uint64(i))
				if err != nil {
					t.Error(err)
					return
				}
				out[w] = append(out[w], ch)
			}
		})
	}
	wg.Wait()
	return out
}

func writeProfiles(t *testing.T, name string) {
	t.Helper()
	dir := os.Getenv("PROFILE_DIR")
	if dir == "" {
		return
	}
	for _, p := range []string{"block", "mutex"} {
		f, err := os.Create(filepath.Join(dir, name+"."+p+".pb.gz"))
		if err != nil {
			t.Fatal(err)
		}
		_ = pprof.Lookup(p).WriteTo(f, 0)
		_ = f.Close()
	}
}

func resetProfiles() {
	runtime.SetBlockProfileRate(0)
	runtime.SetMutexProfileFraction(0)
	runtime.SetBlockProfileRate(1)
	runtime.SetMutexProfileFraction(1)
}

const (
	workers   = 64
	perWorker = 300
)

// TestPutContention compares Put latency when all chunks land in one PO bin
// (what a full node sees: half of its reserve is in bin == radius) with the
// same load spread over 8 bins and with a single writer.
func TestPutContention(t *testing.T) {
	cases := []struct {
		name    string
		workers int
		bins    func(w, i int) int
	}{
		{"1writer_samebin", 1, func(w, i int) int { return 0 }},
		{"64writers_samebin", workers, func(w, i int) int { return 0 }},
		{"64writers_8bins", workers, func(w, i int) int { return w % 8 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newDiskReserve(t)
			per := perWorker
			if c.workers == 1 {
				per = workers * perWorker / 4
			}
			chunks := gen(t, e, c.workers, per, c.bins)
			resetProfiles()
			lat, wall := runPuts(t, e, chunks)
			writeProfiles(t, c.name)
			t.Logf("%s: %v", c.name, summarize(lat, wall, e))
		})
	}
}

// TestPutDuringEviction measures Put latency for a batch while that batch is
// being evicted (EvictBatchBin holds the batch lock for the whole eviction).
func TestPutDuringEviction(t *testing.T) {
	e := newDiskReserve(t)
	hot := make([]byte, 32)
	_, _ = rand.Read(hot)

	const evictN = 20000
	for i := range evictN {
		if err := e.r.Put(context.Background(), mustChunkAt(t, e.base, 0, hot, uint64(i))); err != nil {
			t.Fatal(err)
		}
	}

	// puts into the hot batch (bin 1, not evicted) and into other batches.
	hotPuts := make([]swarm.Chunk, 200)
	for i := range hotPuts {
		hotPuts[i] = mustChunkAt(t, e.base, 1, hot, uint64(evictN+i))
	}
	other := gen(t, e, 8, 200, func(w, i int) int { return 1 })

	resetProfiles()
	evictDone := make(chan time.Duration, 1)
	go func() {
		s := time.Now()
		n, err := e.r.EvictBatchBin(context.Background(), hot, math.MaxInt, 1)
		if err != nil || n != evictN {
			t.Errorf("evicted %d err %v", n, err)
		}
		evictDone <- time.Since(s)
	}()
	time.Sleep(10 * time.Millisecond) // let eviction take the batch lock; measurement only

	var wg sync.WaitGroup
	var hotLat, otherLat []time.Duration
	var hotWall, otherWall time.Duration
	wg.Go(func() { hotLat, hotWall = runPuts(t, e, [][]swarm.Chunk{hotPuts}) })
	wg.Go(func() { otherLat, otherWall = runPuts(t, e, other) })
	wg.Wait()
	writeProfiles(t, "eviction")
	t.Logf("eviction of %d chunks took %v", evictN, <-evictDone)
	t.Logf("hot batch puts:   %v", summarize(hotLat, hotWall, e))
	t.Logf("other batch puts: %v", summarize(otherLat, otherWall, e))
}

// TestPutScaling sweeps the number of concurrent writers into one bin. If Put
// is only serialized, throughput stays flat and latency grows linearly with
// writers. If throughput drops as writers grow, waiting itself costs work.
func TestPutScaling(t *testing.T) {
	const total = 16384
	for _, w := range []int{1, 16, 64, 256, 1024} {
		t.Run(fmt.Sprintf("writers_%d", w), func(t *testing.T) {
			e := newDiskReserve(t)
			chunks := gen(t, e, w, total/w, func(int, int) int { return 0 })
			lat, wall := runPuts(t, e, chunks)
			t.Logf("writers=%4d %v", w, summarize(lat, wall, e))
		})
	}
}

// TestMultexScaling isolates the lock: W goroutines take the same key and
// hold it for a fixed time; optional "noise" goroutines lock/unlock their own
// distinct keys (like Get/Put on other batches). Control: sync.Mutex.
func TestMultexScaling(t *testing.T) {
	const (
		hold   = 50 * time.Microsecond
		perOp  = 200
		noiseN = 256
	)
	spin := func(d time.Duration) {
		for s := time.Now(); time.Since(s) < d; {
		}
	}
	type locker struct {
		lock, unlock func(string)
	}
	run := func(l locker, w int, noise bool) (time.Duration, time.Duration, float64) {
		stop := make(chan struct{})
		var nwg sync.WaitGroup
		if noise {
			for n := range noiseN {
				nwg.Go(func() {
					key := fmt.Sprintf("other-%d", n)
					for {
						select {
						case <-stop:
							return
						default:
						}
						l.lock(key)
						l.unlock(key)
					}
				})
			}
		}
		var (
			mu  sync.Mutex
			all []time.Duration
			wg  sync.WaitGroup
		)
		start := time.Now()
		for range w {
			wg.Go(func() {
				lat := make([]time.Duration, 0, perOp)
				for range perOp {
					s := time.Now()
					l.lock("bin-0")
					lat = append(lat, time.Since(s))
					spin(hold)
					l.unlock("bin-0")
				}
				mu.Lock()
				all = append(all, lat...)
				mu.Unlock()
			})
		}
		wg.Wait()
		wall := time.Since(start)
		close(stop)
		nwg.Wait()
		slices.Sort(all)
		return all[len(all)*99/100], all[len(all)-1], float64(len(all)) / wall.Seconds()
	}

	mx := multex.New()
	var smu sync.Mutex
	locks := map[string]locker{
		"multex":     {mx.Lock, mx.Unlock},
		"sync.Mutex": {func(string) { smu.Lock() }, func(string) { smu.Unlock() }},
	}
	for _, name := range []string{"sync.Mutex", "multex"} {
		for _, noise := range []bool{false, true} {
			if name == "sync.Mutex" && noise {
				continue // a single sync.Mutex has no other keys
			}
			for _, w := range []int{16, 256, 1024} {
				p99, maxW, ops := run(locks[name], w, noise)
				t.Logf("%-10s noise=%-5v waiters=%4d ops/s=%6.0f (ideal %.0f) wait_p99=%v wait_max=%v",
					name, noise, w, ops, float64(time.Second/hold), p99.Round(time.Microsecond), maxW.Round(time.Microsecond))
			}
		}
	}
}

type hasKey struct {
	addr          swarm.Address
	batch, sthash []byte
}

// prepopulate puts n chunks spread over bins 0..7 so leveldb has real levels
// and Has has to read from disk, and returns their Has keys.
func prepopulate(t *testing.T, e *env, n int) []hasKey {
	t.Helper()
	const w = 32
	chunks := gen(t, e, w, n/w, func(w, i int) int { return (w + i) % 8 })
	if _, wall := runPuts(t, e, chunks); wall == 0 {
		t.Fatal("no puts")
	}
	keys := make([]hasKey, 0, n)
	for _, cs := range chunks {
		for _, ch := range cs {
			h, err := ch.Stamp().Hash()
			if err != nil {
				t.Fatal(err)
			}
			keys = append(keys, hasKey{ch.Address(), ch.Stamp().BatchID(), h})
		}
	}
	return keys
}

// TestPutWithHas measures Put latency while H goroutines call Reserve.Has in
// a loop, as pullsync does for every offered chunk (pullsync.go ReserveHas).
// HAS_N picks H (default 0), HAS_HIT=1 makes Has look up stored keys.
func TestPutWithHas(t *testing.T) {
	hasN := 0
	if v := os.Getenv("HAS_N"); v != "" {
		_, _ = fmt.Sscan(v, &hasN)
	}
	writers := 64
	if v := os.Getenv("WRITERS"); v != "" {
		_, _ = fmt.Sscan(v, &writers)
	}
	hit := os.Getenv("HAS_HIT") == "1"

	e := newDiskReserve(t)
	keys := prepopulate(t, e, 200_000)
	chunks := gen(t, e, writers, 16384/writers, func(int, int) int { return 0 })

	var (
		stop    = make(chan struct{})
		hwg     sync.WaitGroup
		hmu     sync.Mutex
		hasLat  []time.Duration
		hasOps  atomic.Int64
		missing = make([]hasKey, 4096)
	)
	for i := range missing {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		missing[i] = hasKey{swarm.RandAddress(t), b, b}
	}
	for h := range hasN {
		hwg.Go(func() {
			var lat []time.Duration
			for i := h; ; i++ {
				select {
				case <-stop:
					hmu.Lock()
					hasLat = append(hasLat, lat...)
					hmu.Unlock()
					return
				default:
				}
				k := missing[i%len(missing)]
				if hit {
					k = keys[(i*7919)%len(keys)]
				}
				s := time.Now()
				if _, err := e.r.Has(k.addr, k.batch, k.sthash); err != nil {
					t.Error(err)
					return
				}
				hasOps.Add(1)
				if i%32 == 0 { // sample latencies to bound memory
					lat = append(lat, time.Since(s))
				}
			}
		})
	}
	lat, wall := runPuts(t, e, chunks)
	close(stop)
	hwg.Wait()
	t.Logf("writers=%d has_goroutines=%d has_hit=%v puts: %v", writers, hasN, hit, summarize(lat, wall, e))
	if len(hasLat) > 0 {
		hs := summarize(hasLat, wall, e)
		t.Logf("  has: n=%d has/s=%.0f sampled p50=%v p99=%v max=%v", hasOps.Load(), float64(hasOps.Load())/wall.Seconds(), hs.p50, hs.p99, hs.max)
	}
}
