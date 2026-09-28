// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// SamplingViewer is implemented by storages that can serve reserve sampling
// reads without a retrieval index lookup per chunk.
type SamplingViewer interface {
	NewSamplingView(ctx context.Context, anchor []byte, depth uint8) (SamplingView, error)
}

// SamplingView reads chunks within depth of anchor using the locations the
// retrieval index held when the view was opened. GetInto is safe for
// concurrent use. Close must be called when sampling ends.
type SamplingView interface {
	storage.GetterInto
	// Len returns the number of addresses in the location table.
	Len() int
	// Misses returns how many reads fell back to the retrieval index.
	Misses() int64
	Close() error
}

var _ SamplingViewer = (*store)(nil)

// NewSamplingView holds released sharky slots and only then snapshots the
// locations. A slot referenced by the snapshot is released, if at all, after
// its index entry is committed away (see transaction.Commit), which is after
// the hold started, so its content stays in place until Close.
func (s *store) NewSamplingView(ctx context.Context, anchor []byte, depth uint8) (SamplingView, error) {
	release := s.sharky.Hold()
	table, err := chunkstore.BuildLocationTable(ctx, s.IndexStore(), anchor, depth)
	if err != nil {
		release()
		return nil, err
	}
	return &samplingView{store: s, table: table, release: release}, nil
}

type samplingView struct {
	store   *store
	table   *chunkstore.LocationTable
	release func()
	misses  atomic.Int64
}

func (v *samplingView) GetInto(ctx context.Context, addr swarm.Address, buf []byte) (n int, err error) {
	loc, ok := v.table.Lookup(addr)
	if !ok {
		v.misses.Add(1)
		return v.store.ChunkStore().GetInto(ctx, addr, buf)
	}

	defer handleMetric("sampling_view_get", v.store.metrics)(&err)
	n = int(loc.Length)
	if len(buf) < n {
		return 0, fmt.Errorf("sampling view: buffer too small: %d < %d", len(buf), n)
	}
	if err = v.store.sharky.Read(ctx, loc, buf[:n]); err != nil {
		return 0, fmt.Errorf("sampling view: read %s at %v: %w", addr, loc, err)
	}
	return n, nil
}

func (v *samplingView) Len() int { return v.table.Len() }

func (v *samplingView) Misses() int64 { return v.misses.Load() }

func (v *samplingView) Close() error {
	v.release()
	return nil
}
