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

// sharkyReader is the part of sharky.Store a SamplingView reads through.
type sharkyReader interface {
	Read(ctx context.Context, loc sharky.Location, buf []byte) error
	Watch(fn func(sharky.Location)) (stop func())
}

// SamplingView reads chunks within depth of anchor using the locations the
// retrieval index held when the view was opened. A location whose sharky slot
// has been released since is not trusted; the chunk is then read through the
// retrieval index, as the chunk store would. GetInto is safe for concurrent
// use. Close must be called when sampling ends.
type SamplingView struct {
	sharky   sharkyReader
	chunks   storage.ReadOnlyChunkStore // fallback for reads the table cannot serve
	table    *chunkstore.LocationTable
	released *releasedSlots
	stop     func()
	misses   atomic.Int64
}

// NewSamplingView opens a SamplingView over the chunks within depth of anchor.
func (s *store) NewSamplingView(ctx context.Context, anchor []byte, depth uint8) (*SamplingView, error) {
	return newSamplingView(ctx, s.sharky, s, anchor, depth)
}

// newSamplingView watches sharky releases and only then snapshots the
// locations. An index entry is committed away before its slot is released (see
// transaction.Commit), so a slot the snapshot references that is freed later
// is recorded before a write can reuse it.
func newSamplingView(ctx context.Context, sh sharkyReader, st ReadOnlyStore, anchor []byte, depth uint8) (*SamplingView, error) {
	released := new(releasedSlots)
	stop := sh.Watch(released.add)

	table, err := chunkstore.BuildLocationTable(ctx, st.IndexStore(), anchor, depth)
	if err != nil {
		stop()
		return nil, err
	}
	released.publish(table.SlotLimits())

	return &SamplingView{
		sharky:   sh,
		chunks:   st.ChunkStore(),
		table:    table,
		released: released,
		stop:     stop,
	}, nil
}

func (v *SamplingView) GetInto(ctx context.Context, addr swarm.Address, buf []byte) (int, error) {
	if loc, ok := v.table.Lookup(addr); ok && !v.released.contains(loc) {
		n, err := v.readAt(ctx, addr, loc, buf)
		// A slot released during the read may already hold another chunk.
		if !v.released.contains(loc) {
			return n, err
		}
	}
	v.misses.Add(1)
	return v.chunks.GetInto(ctx, addr, buf)
}

func (v *SamplingView) readAt(ctx context.Context, addr swarm.Address, loc sharky.Location, buf []byte) (int, error) {
	n := int(loc.Length)
	if len(buf) < n {
		return 0, fmt.Errorf("sampling view: buffer too small: %d < %d", len(buf), n)
	}
	if err := v.sharky.Read(ctx, loc, buf[:n]); err != nil {
		return 0, fmt.Errorf("sampling view: read %s at %v: %w", addr, loc, err)
	}
	return n, nil
}

// Len returns the number of addresses in the location table.
func (v *SamplingView) Len() int { return v.table.Len() }

// Misses returns how many reads fell back to the retrieval index.
func (v *SamplingView) Misses() int64 { return v.misses.Load() }

func (v *SamplingView) Close() error {
	v.stop()
	return nil
}
