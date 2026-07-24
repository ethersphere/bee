// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sim

import (
	"context"
	"time"

	"github.com/ethersphere/bee/v2/pkg/pullsync"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var _ pullsync.Interface = (*syncWrap)(nil)

// SyncEvent is emitted for every completed client-side Sync round. It captures
// the ~1s coalescing wait via Dur.
type SyncEvent struct {
	Peer    swarm.Address
	Bin     uint8
	Start   uint64
	Topmost uint64
	Count   int
	Dur     time.Duration
	Err     error
}

// syncWrap decorates a node's Syncer with per-round instrumentation before it
// is handed to the puller. GetCursors is passed through untouched.
type syncWrap struct {
	inner  pullsync.Interface
	onSync func(SyncEvent)
}

func newSyncWrap(inner pullsync.Interface, onSync func(SyncEvent)) *syncWrap {
	return &syncWrap{inner: inner, onSync: onSync}
}

func (w *syncWrap) Sync(ctx context.Context, peer swarm.Address, bin uint8, start uint64) (uint64, int, error) {
	t0 := time.Now()
	top, count, err := w.inner.Sync(ctx, peer, bin, start)
	if w.onSync != nil {
		w.onSync(SyncEvent{
			Peer:    peer,
			Bin:     bin,
			Start:   start,
			Topmost: top,
			Count:   count,
			Dur:     time.Since(t0),
			Err:     err,
		})
	}
	return top, count, err
}

func (w *syncWrap) GetCursors(ctx context.Context, peer swarm.Address) ([]uint64, uint64, error) {
	return w.inner.GetCursors(ctx, peer)
}
