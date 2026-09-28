// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	test "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"golang.org/x/sync/errgroup"
)

// newViewStorage uses a single sharky shard so that a write after a release
// would take the released slot if nothing held it.
func newViewStorage(t *testing.T) (transaction.Storage, transaction.SamplingViewer) {
	t.Helper()
	sh, err := sharky.New(&dirFS{basedir: t.TempDir()}, 1, swarm.SocMaxChunkSize)
	if err != nil {
		t.Fatal(err)
	}
	idx, _, err := leveldbstore.New("", nil)
	if err != nil {
		t.Fatal(err)
	}
	st := transaction.NewStorage(sh, idx)
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Fatal(err)
		}
	})
	viewer, ok := st.(transaction.SamplingViewer)
	if !ok {
		t.Fatal("storage does not implement SamplingViewer")
	}
	return st, viewer
}

func putViewChunks(t *testing.T, st transaction.Storage, chs ...swarm.Chunk) {
	t.Helper()
	err := st.Run(context.Background(), func(s transaction.Store) error {
		for _, ch := range chs {
			if err := s.ChunkStore().Put(context.Background(), ch); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func openView(t *testing.T, viewer transaction.SamplingViewer) transaction.SamplingView {
	t.Helper()
	view, err := viewer.NewSamplingView(context.Background(), swarm.ZeroAddress.Bytes(), 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = view.Close() })
	return view
}

func assertViewReads(t *testing.T, view transaction.SamplingView, addr swarm.Address, want []byte) {
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

	st, viewer := newViewStorage(t)
	chs := test.GenerateTestRandomChunks(10)
	putViewChunks(t, st, chs...)

	view := openView(t, viewer)
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

	st, viewer := newViewStorage(t)
	view := openView(t, viewer)

	ch := test.GenerateTestRandomChunk()
	putViewChunks(t, st, ch)

	assertViewReads(t, view, ch.Address(), ch.Data())
	if view.Misses() != 1 {
		t.Fatalf("misses %d, want 1", view.Misses())
	}
}

func TestSamplingViewKeepsDeletedChunkContent(t *testing.T) {
	t.Parallel()

	st, viewer := newViewStorage(t)
	ch := test.GenerateTestRandomChunk()
	putViewChunks(t, st, ch)

	view := openView(t, viewer)

	err := st.Run(context.Background(), func(s transaction.Store) error {
		return s.ChunkStore().Delete(context.Background(), ch.Address())
	})
	if err != nil {
		t.Fatal(err)
	}
	// Without the hold, one of these would take the released slot.
	putViewChunks(t, st, test.GenerateTestRandomChunks(16)...)

	assertViewReads(t, view, ch.Address(), ch.Data())
}

func TestSamplingViewReadsSnapshotVersionOfReplacedChunk(t *testing.T) {
	t.Parallel()

	st, viewer := newViewStorage(t)
	addr := swarm.RandAddress(t)
	v1 := swarm.NewChunk(addr, []byte("snapshot version"))
	v2 := swarm.NewChunk(addr, []byte("version written during the round"))
	putViewChunks(t, st, v1)

	view := openView(t, viewer)

	err := st.Run(context.Background(), func(s transaction.Store) error {
		return s.ChunkStore().Replace(context.Background(), v2, false)
	})
	if err != nil {
		t.Fatal(err)
	}
	putViewChunks(t, st, test.GenerateTestRandomChunks(16)...)

	assertViewReads(t, view, addr, v1.Data())

	if err := view.Close(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, swarm.SocMaxChunkSize)
	n, err := st.ChunkStore().GetInto(context.Background(), addr, buf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf[:n], v2.Data()) {
		t.Fatal("chunk store must return the replaced version after the view closes")
	}
}

func TestSamplingViewConcurrentReadsAndWrites(t *testing.T) {
	t.Parallel()

	st, viewer := newViewStorage(t)
	chs := test.GenerateTestRandomChunks(64)
	putViewChunks(t, st, chs...)

	view := openView(t, viewer)

	var g errgroup.Group
	for range 4 {
		g.Go(func() error {
			buf := make([]byte, swarm.SocMaxChunkSize)
			for _, ch := range chs {
				n, err := view.GetInto(context.Background(), ch.Address(), buf)
				if err != nil {
					return err
				}
				if !bytes.Equal(buf[:n], ch.Data()) {
					t.Errorf("chunk %s: content changed under the view", ch.Address())
				}
			}
			return nil
		})
	}
	g.Go(func() error {
		for _, ch := range chs[:32] {
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
