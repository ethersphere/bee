// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/ethersphere/bee/v2/pkg/sharky"
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
// retrieval index held when the view was opened. A location whose sharky slot
// has been released since is not trusted; the chunk is then read through the
// retrieval index, as the chunk store would. GetInto is safe for concurrent
// use. Close must be called when sampling ends.
type SamplingView interface {
	storage.GetterInto
	// Len returns the number of addresses in the location table.
	Len() int
	// Misses returns how many reads fell back to the retrieval index.
	Misses() int64
	Close() error
}

var _ SamplingViewer = (*store)(nil)

// NewSamplingView watches sharky releases and only then snapshots the
// locations. An index entry is committed away before its slot is released (see
// transaction.Commit), so a slot the snapshot references that is freed later
// is recorded before a write can reuse it.
func (s *store) NewSamplingView(ctx context.Context, anchor []byte, depth uint8) (SamplingView, error) {
	released := new(releasedSlots)
	stop := s.sharky.Watch(released.add)
	opened := false
	defer func() {
		if !opened {
			stop()
		}
	}()

	table, err := chunkstore.BuildLocationTable(ctx, s.IndexStore(), anchor, depth)
	if err != nil {
		return nil, err
	}
	released.publish(table.SlotLimits())

	opened = true
	return &samplingView{
		store:    s,
		table:    table,
		released: released,
		stop:     stop,
		read:     s.sharky.Read,
	}, nil
}

type samplingView struct {
	store    *store
	table    *chunkstore.LocationTable
	released *releasedSlots
	stop     func()
	read     func(context.Context, sharky.Location, []byte) error
	misses   atomic.Int64
}

func (v *samplingView) GetInto(ctx context.Context, addr swarm.Address, buf []byte) (int, error) {
	if loc, ok := v.table.Lookup(addr); ok && !v.released.contains(loc) {
		n, err := v.readAt(ctx, addr, loc, buf)
		// A slot released during the read may already hold another chunk.
		if !v.released.contains(loc) {
			return n, err
		}
	}
	v.misses.Add(1)
	return v.store.ChunkStore().GetInto(ctx, addr, buf)
}

func (v *samplingView) readAt(ctx context.Context, addr swarm.Address, loc sharky.Location, buf []byte) (int, error) {
	n := int(loc.Length)
	if len(buf) < n {
		return 0, fmt.Errorf("sampling view: buffer too small: %d < %d", len(buf), n)
	}
	if err := v.read(ctx, loc, buf[:n]); err != nil {
		return 0, fmt.Errorf("sampling view: read %s at %v: %w", addr, loc, err)
	}
	return n, nil
}

func (v *samplingView) Len() int { return v.table.Len() }

func (v *samplingView) Misses() int64 { return v.misses.Load() }

func (v *samplingView) Close() error {
	v.stop()
	return nil
}
