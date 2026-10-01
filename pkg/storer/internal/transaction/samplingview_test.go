// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	test "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"golang.org/x/sync/errgroup"
)

// newViewStorage uses a single sharky shard so that a write after a release takes the released slot.
func newViewStorage(tb testing.TB) (transaction.Storage, *sharky.Store) {
	tb.Helper()
	sh, err := sharky.New(&dirFS{basedir: tb.TempDir()}, 1, swarm.SocMaxChunkSize)
	if err != nil {
		tb.Fatal(err)
	}
	idx, _, err := leveldbstore.New("", nil)
	if err != nil {
		tb.Fatal(err)
	}
	st := transaction.NewStorage(sh, idx)
	tb.Cleanup(func() {
		if err := st.Close(); err != nil {
			tb.Fatal(err)
		}
	})
	return st, sh
}

func putViewChunks(tb testing.TB, st transaction.Storage, chs ...swarm.Chunk) {
	tb.Helper()
	err := st.Run(context.Background(), func(s transaction.Store) error {
		for _, ch := range chs {
			if err := s.ChunkStore().Put(context.Background(), ch); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
}

func deleteViewChunk(tb testing.TB, st transaction.Storage, addr swarm.Address) {
	tb.Helper()
	err := st.Run(context.Background(), func(s transaction.Store) error {
		return s.ChunkStore().Delete(context.Background(), addr)
	})
	if err != nil {
		tb.Fatal(err)
	}
}

func assertViewNotFound(t *testing.T, view *transaction.SamplingView, addr swarm.Address) {
	t.Helper()
	_, err := view.GetInto(context.Background(), addr, make([]byte, swarm.SocMaxChunkSize))
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("chunk %s: got error %v, want %v", addr, err, storage.ErrNotFound)
	}
}

func openView(tb testing.TB, sh transaction.Sharky, st transaction.ReadOnlyStore) *transaction.SamplingView {
	tb.Helper()
	view, err := transaction.NewSamplingView(context.Background(), sh, st, swarm.ZeroAddress.Bytes(), 0, 0)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = view.Close() })
	return view
}

// afterScanStore calls afterScan once the location table scan of its index is done.
type afterScanStore struct {
	transaction.ReadOnlyStore
	afterScan func()
}

func (s afterScanStore) IndexStore() storage.Reader {
	return afterScanReader{s.ReadOnlyStore.IndexStore(), s.afterScan}
}

type afterScanReader struct {
	storage.Reader
	afterScan func()
}

func (r afterScanReader) Iterate(q storage.Query, fn storage.IterateFn) error {
	err := r.Reader.Iterate(q, fn)
	r.afterScan()
	return err
}

// afterReadSharky calls afterRead after each sharky read.
type afterReadSharky struct {
	transaction.Sharky
	afterRead func()
}

func (s afterReadSharky) Read(ctx context.Context, loc sharky.Location, buf []byte) error {
	err := s.Sharky.Read(ctx, loc, buf)
	s.afterRead()
	return err
}

func assertViewReads(t *testing.T, view *transaction.SamplingView, addr swarm.Address, want []byte) {
	t.Helper()
	buf := make([]byte, swarm.SocMaxChunkSize)
	n, err := view.GetInto(context.Background(), addr, buf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf[:n], want) {
		t.Fatalf("chunk %s: read %d bytes that differ from the expected %d", addr, n, len(want))
	}
}

func TestSamplingViewReadsSnapshotLocations(t *testing.T) {
	t.Parallel()

	st, sh := newViewStorage(t)
	chs := test.GenerateTestRandomChunks(10)
	putViewChunks(t, st, chs...)

	view := openView(t, sh, st)
	if view.Len() != len(chs) {
		t.Fatalf("table size %d, want %d", view.Len(), len(chs))
	}
	for _, ch := range chs {
		assertViewReads(t, view, ch.Address(), ch.Data())
	}
	if view.Misses() != 0 {
		t.Fatalf("misses %d, want 0", view.Misses())
	}
}

func TestSamplingViewFallsBackForNewChunks(t *testing.T) {
	t.Parallel()

	st, sh := newViewStorage(t)
	view := openView(t, sh, st)

	ch := test.GenerateTestRandomChunk()
	putViewChunks(t, st, ch)

	assertViewReads(t, view, ch.Address(), ch.Data())
	if view.Misses() != 1 {
		t.Fatalf("misses %d, want 1", view.Misses())
	}
}

func TestSamplingViewHonorsReleaseDuringScan(t *testing.T) {
	t.Parallel()

	st, sh := newViewStorage(t)
	ch := test.GenerateTestRandomChunk()
	putViewChunks(t, st, ch)

	// Release the chunk's slot and reuse it while the table is being built.
	view := openView(t, sh, afterScanStore{st, func() {
		deleteViewChunk(t, st, ch.Address())
		putViewChunks(t, st, test.GenerateTestRandomChunks(16)...)
	}})

	assertViewNotFound(t, view, ch.Address())
	if view.Misses() != 1 {
		t.Fatalf("misses %d, want 1", view.Misses())
	}
}

func TestSamplingViewFallsBackForReleasedSlots(t *testing.T) {
	t.Parallel()

	st, sh := newViewStorage(t)
	ch := test.GenerateTestRandomChunk()
	putViewChunks(t, st, ch)

	view := openView(t, sh, st)

	deleteViewChunk(t, st, ch.Address())
	putViewChunks(t, st, test.GenerateTestRandomChunks(16)...) // one takes the released slot

	assertViewNotFound(t, view, ch.Address())
	if view.Misses() != 1 {
		t.Fatalf("misses %d, want 1", view.Misses())
	}
}

func TestSamplingViewFallsBackForSlotReleasedDuringRead(t *testing.T) {
	t.Parallel()

	st, sh := newViewStorage(t)
	ch := test.GenerateTestRandomChunk()
	putViewChunks(t, st, ch)

	view := openView(t, afterReadSharky{sh, func() {
		deleteViewChunk(t, st, ch.Address())
		putViewChunks(t, st, test.GenerateTestRandomChunks(16)...)
	}}, st)

	assertViewNotFound(t, view, ch.Address())
}

func TestSamplingViewCanceledContext(t *testing.T) {
	t.Parallel()

	st, sh := newViewStorage(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := transaction.NewSamplingView(ctx, sh, st, swarm.ZeroAddress.Bytes(), 0, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("got error %v, want %v", err, context.Canceled)
	}
}

func TestSamplingViewReadsCurrentVersionOfReplacedChunk(t *testing.T) {
	t.Parallel()

	st, sh := newViewStorage(t)
	addr := swarm.RandAddress(t)
	v1 := swarm.NewChunk(addr, []byte("version in the table"))
	v2 := swarm.NewChunk(addr, []byte("version written during the round"))
	putViewChunks(t, st, v1)

	view := openView(t, sh, st)

	err := st.Run(context.Background(), func(s transaction.Store) error {
		return s.ChunkStore().Replace(context.Background(), v2, false)
	})
	if err != nil {
		t.Fatal(err)
	}
	putViewChunks(t, st, test.GenerateTestRandomChunks(16)...)

	assertViewReads(t, view, addr, v2.Data())
	if view.Misses() != 1 {
		t.Fatalf("misses %d, want 1", view.Misses())
	}
}

func TestSamplingViewConcurrentReadsAndWrites(t *testing.T) {
	t.Parallel()

	st, sh := newViewStorage(t)
	const deleted = 32
	chs := test.GenerateTestRandomChunks(64)
	putViewChunks(t, st, chs...)

	view := openView(t, sh, st)

	var g errgroup.Group
	for range 4 {
		g.Go(func() error {
			buf := make([]byte, swarm.SocMaxChunkSize)
			for i, ch := range chs {
				n, err := view.GetInto(context.Background(), ch.Address(), buf)
				switch {
				case errors.Is(err, storage.ErrNotFound) && i < deleted:
					// deleted by the writer; the live store agrees
				case err != nil:
					return err
				case !bytes.Equal(buf[:n], ch.Data()):
					t.Errorf("chunk %s: read another chunk's content", ch.Address())
				}
			}
			return nil
		})
	}
	g.Go(func() error {
		for _, ch := range chs[:deleted] {
			err := st.Run(context.Background(), func(s transaction.Store) error {
				return s.ChunkStore().Delete(context.Background(), ch.Address())
			})
			if err != nil {
				return err
			}
			err = st.Run(context.Background(), func(s transaction.Store) error {
				return s.ChunkStore().Put(context.Background(), test.GenerateTestRandomChunk())
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkSamplingViewGetInto(b *testing.B) {
	st, sh := newViewStorage(b)
	chs := test.GenerateTestRandomChunks(1000)
	putViewChunks(b, st, chs...)
	view := openView(b, sh, st)
	buf := make([]byte, swarm.SocMaxChunkSize)
	ctx := context.Background()

	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if _, err := view.GetInto(ctx, chs[i].Address(), buf); err != nil {
			b.Fatal(err)
		}
		if i++; i == len(chs) {
			i = 0
		}
	}
}
