// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	kademlia "github.com/ethersphere/bee/v2/pkg/topology/mock"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// benchFillerItem stands in for the other per-chunk index entries of a real
// storer (retrieval index, chunk stamp, stamp index), so the leveldb the fill
// works on has a realistic size.
type benchFillerItem struct {
	ns, id string
	val    []byte
}

func (b *benchFillerItem) Namespace() string          { return b.ns }
func (b *benchFillerItem) ID() string                 { return b.id }
func (b *benchFillerItem) String() string             { return b.ns + "/" + b.id }
func (b *benchFillerItem) Clone() storage.Item        { c := *b; return &c }
func (b *benchFillerItem) Marshal() ([]byte, error)   { return b.val, nil }
func (b *benchFillerItem) Unmarshal(buf []byte) error { b.val = buf; return nil }
func randomBytes(n int) []byte                        { b := make([]byte, n); _, _ = rand.Read(b); return b }

func benchEnvInts(name string, def []int) ([]int, error) {
	v := os.Getenv(name)
	if v == "" {
		return def, nil
	}
	var out []int
	for _, f := range strings.Split(v, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		out = append(out, n)
	}
	return out, nil
}

func dirSize(tb testing.TB, dir string) int64 {
	tb.Helper()
	var size int64
	// leveldb compactions delete table files concurrently: skip vanished ones
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		size += info.Size()
		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
	return size
}

func loadAvg() string {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return "n/a"
	}
	return strings.Join(strings.Fields(string(b))[:3], " ")
}

// BenchmarkAddrIndexFill measures the address index start-up work on a
// leveldb store opened with the storer's options, for gate G4 (blocking or
// background fill): the fill of an empty index, the digest count, a start
// with a valid marker, the synchronous drop of a full index, the compaction
// of its key range and the refill, plus one full IterateAddr scan.
//
// The store is populated directly (untimed) with n ChunkBinItems and
// BatchRadiusItems, the bin IDs and the epoch, plus three filler entries per
// chunk standing in for the storer's other per-chunk index entries; chunk
// data is not written (the fill does not read it). Run one iteration per size:
//
//	ADDRINDEX_BENCH_N=100000,1000000,4194304 ADDRINDEX_BENCH_DIR=/var/tmp/x \
//	  go test -run - -bench AddrIndexFill -benchtime 1x -timeout 3h ./pkg/storer/internal/reserve/
//
// ADDRINDEX_BENCH_DIR should be on a disk (t.TempDir is often tmpfs).
// ADDRINDEX_BENCH_FILLER=0 leaves out the filler entries.
func BenchmarkAddrIndexFill(b *testing.B) {
	sizes, err := benchEnvInts("ADDRINDEX_BENCH_N", []int{20_000})
	if err != nil {
		b.Fatal(err)
	}
	filler := os.Getenv("ADDRINDEX_BENCH_FILLER") != "0"

	for _, n := range sizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			for range b.N {
				benchAddrIndexFill(b, n, filler)
			}
		})
	}
}

func benchAddrIndexFill(b *testing.B, n int, filler bool) {
	b.Helper()
	b.StopTimer()
	ctx := context.Background()

	base := os.Getenv("ADDRINDEX_BENCH_DIR")
	if base == "" {
		base = b.TempDir()
	}
	dir, err := os.MkdirTemp(base, "addrindex-bench-")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)
	ldbDir := filepath.Join(dir, "indexstore")

	sharkyStore, err := sharky.New(&reserveDirFS{basedir: dir}, 1, swarm.SocMaxChunkSize)
	if err != nil {
		b.Fatal(err)
	}
	// the storer's options (initStore), with seeks compaction disabled as
	// the node's --db-disable-seeks-compaction default
	store, _, err := leveldbstore.New(ldbDir, &opt.Options{
		OpenFilesCacheCapacity: 256,
		BlockCacheCapacity:     32 * 1024 * 1024,
		WriteBuffer:            32 * 1024 * 1024,
		DisableSeeksCompaction: true,
		CompactionL0Trigger:    8,
		Filter:                 filter.NewBloomFilter(64),
	})
	if err != nil {
		b.Fatal(err)
	}
	st := transaction.NewStorage(sharkyStore, store)
	defer st.Close()

	baseAddr := swarm.RandAddress(b)
	batches := make([][]byte, 100)
	for i := range batches {
		batches[i] = randomBytes(swarm.HashSize)
	}

	loadStart := loadAvg()
	start := time.Now()
	var binIDs [swarm.MaxBins]uint64
	const page = 10_000
	for done := 0; done < n; done += page {
		err := st.Run(ctx, func(s transaction.Store) error {
			for i := done; i < min(done+page, n); i++ {
				addr := swarm.NewAddress(randomBytes(swarm.HashSize))
				bin := swarm.Proximity(baseAddr.Bytes(), addr.Bytes())
				binIDs[bin]++
				stampHash := randomBytes(swarm.HashSize)
				batchID := batches[i%len(batches)]
				err := s.IndexStore().Put(&reserve.BatchRadiusItem{Bin: bin, BatchID: batchID, Address: addr, BinID: binIDs[bin], StampHash: stampHash})
				if err != nil {
					return err
				}
				err = s.IndexStore().Put(&reserve.ChunkBinItem{Bin: bin, BinID: binIDs[bin], Address: addr, BatchID: batchID, ChunkType: swarm.ChunkTypeContentAddressed, StampHash: stampHash})
				if err != nil {
					return err
				}
				if filler {
					for _, ns := range []string{"benchFillerA", "benchFillerB", "benchFillerC"} {
						if err := s.IndexStore().Put(&benchFillerItem{ns: ns, id: string(randomBytes(64)), val: randomBytes(80)}); err != nil {
							return err
						}
					}
				}
			}
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}
	}
	err = st.Run(ctx, func(s transaction.Store) error {
		for bin, id := range binIDs {
			if err := s.IndexStore().Put(&reserve.BinItem{Bin: uint8(bin), BinID: id}); err != nil {
				return err
			}
		}
		return s.IndexStore().Put(&reserve.EpochItem{Timestamp: uint64(time.Now().Unix())})
	})
	if err != nil {
		b.Fatal(err)
	}
	populate := time.Since(start)
	sizePopulated := dirSize(b, ldbDir)

	r, err := reserve.New(baseAddr, st, n, kademlia.NewTopologyDriver(), log.Noop, reserve.WithAddrIndex(true))
	if err != nil {
		b.Fatal(err)
	}

	timed := func(name string, f func() error) time.Duration {
		b.Helper()
		b.StartTimer()
		start := time.Now()
		err := f()
		took := time.Since(start)
		b.StopTimer()
		if err != nil {
			b.Fatalf("%s: %v", name, err)
		}
		b.ReportMetric(took.Seconds(), name+"_s")
		return took
	}

	// fill of an empty index (fresh enable): the check finds no marker and
	// nothing to drop; the fill ends with the marker (one digest count)
	var filled int
	fill := timed("fill", func() error {
		needsFill, _, err := r.CheckAddrIndex(ctx)
		if err != nil || !needsFill {
			return fmt.Errorf("check: %v, %w", needsFill, err)
		}
		filled, err = r.FillAddrIndex(ctx)
		return err
	})
	if filled != n {
		b.Fatalf("filled %d of %d", filled, n)
	}
	sizeFilled := dirSize(b, ldbDir)

	count := timed("count", func() error {
		_, err := reserve.ReadAddrIndexDigest(ctx, st.IndexStore())
		return err
	})

	// a start with a valid marker
	r2, err := reserve.New(baseAddr, st, n, kademlia.NewTopologyDriver(), log.Noop, reserve.WithAddrIndex(true))
	if err != nil {
		b.Fatal(err)
	}
	checkValid := timed("check_valid", func() error {
		needsFill, _, err := r2.CheckAddrIndex(ctx)
		if err == nil && needsFill {
			err = fmt.Errorf("valid marker not trusted")
		}
		return err
	})

	var scanned int
	scan := timed("scan", func() error {
		return reserve.IterateAddr(st.IndexStore(), swarm.ZeroAddress, 0, nil, func(reserve.ChunkID) (bool, error) {
			scanned++
			return false, nil
		})
	})

	// an enabled start with an invalid marker: synchronous drop, compaction,
	// refill
	err = st.Run(ctx, func(s transaction.Store) error { return s.IndexStore().Delete(&reserve.ChunkAddrValidItem{}) })
	if err != nil {
		b.Fatal(err)
	}
	r3, err := reserve.New(baseAddr, st, n, kademlia.NewTopologyDriver(), log.Noop, reserve.WithAddrIndex(true))
	if err != nil {
		b.Fatal(err)
	}
	var dropped int
	drop := timed("drop", func() error {
		var err error
		_, dropped, err = r3.CheckAddrIndex(ctx)
		return err
	})
	if dropped != n {
		b.Fatalf("dropped %d of %d", dropped, n)
	}
	// one seek into the emptied range, before and after the compaction
	seek := func() error {
		mid := swarm.NewAddress(append([]byte{0x80}, make([]byte, swarm.HashSize-1)...))
		return reserve.IterateAddr(st.IndexStore(), mid, 1, nil, func(reserve.ChunkID) (bool, error) { return true, nil })
	}
	seekDropped := timed("seek_after_drop", seek)
	compact := timed("compact", func() error {
		return store.DB().CompactRange(reserve.AddrIndexKeyRange())
	})
	seekCompacted := timed("seek_after_compact", seek)
	sizeDropped := dirSize(b, ldbDir)
	refill := timed("refill", func() error {
		_, err := r3.FillAddrIndex(ctx)
		return err
	})

	b.ReportMetric(float64(sizePopulated)/(1<<20), "MiB_populated")
	b.ReportMetric(float64(sizeFilled)/(1<<20), "MiB_filled")
	b.ReportMetric(float64(sizeDropped)/(1<<20), "MiB_dropped")
	b.Logf("n=%d filler=%v populate=%s fill=%s count=%s check_valid=%s scan=%s (%d) drop=%s seek_after_drop=%s compact=%s seek_after_compact=%s refill=%s MiB populated=%.0f filled=%.0f after_drop+compact=%.0f load %s -> %s",
		n, filler, populate.Round(time.Millisecond), fill.Round(time.Millisecond), count.Round(time.Millisecond),
		checkValid.Round(time.Millisecond), scan.Round(time.Millisecond), scanned, drop.Round(time.Millisecond),
		seekDropped.Round(time.Microsecond), compact.Round(time.Millisecond), seekCompacted.Round(time.Microsecond), refill.Round(time.Millisecond),
		float64(sizePopulated)/(1<<20), float64(sizeFilled)/(1<<20), float64(sizeDropped)/(1<<20), loadStart, loadAvg())
}
