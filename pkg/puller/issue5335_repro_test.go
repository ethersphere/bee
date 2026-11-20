// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package puller_test

// Throwaway reproduction for https://github.com/ethersphere/bee/issues/5335.
// Measures how often the puller calls Sync when Sync keeps failing with
// count=0 and topmost=0 (for example "read offer: stream reset").

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/puller"
	"github.com/ethersphere/bee/v2/pkg/statestore/leveldb"
	resMock "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	kadMock "github.com/ethersphere/bee/v2/pkg/topology/kademlia/mock"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

type failingSyncer struct {
	calls   atomic.Int64
	err     error
	cursors []uint64
}

func (f *failingSyncer) Sync(ctx context.Context, _ swarm.Address, _ uint8, _ uint64) (uint64, int, error) {
	f.calls.Add(1)
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	return 0, 0, f.err
}

func (f *failingSyncer) GetCursors(context.Context, swarm.Address) ([]uint64, uint64, error) {
	return f.cursors, 0, nil
}

func TestIssue5335PullerRetrySpin(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "sync error, count 0", err: errors.New("read offer: stream reset")},
		{name: "no error, empty offer, topmost 0", err: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			addr := swarm.RandAddress(t)
			syncer := &failingSyncer{err: tc.err, cursors: []uint64{1000, 1000, 1000}}

			st, err := leveldb.NewStateStore(t.TempDir(), log.Noop)
			if err != nil {
				t.Fatal(err)
			}
			kad := kadMock.NewMockKademlia(kadMock.WithEachPeerRevCalls(kadMock.AddrTuple{Addr: addr, PO: 1}))
			p := puller.New(swarm.RandAddress(t), st, kad, resMock.NewReserve(resMock.WithRadius(1)), syncer, nil, log.Noop, puller.Options{Bins: 3})
			p.Start(context.Background())
			testutil.CleanupCloser(t, p, st)

			kad.Trigger()

			// Let workers start, then measure one second.
			time.Sleep(200 * time.Millisecond)
			before := syncer.calls.Load()
			time.Sleep(time.Second)
			perSec := syncer.calls.Load() - before

			// 1 peer, bins 1 and 2, historical + live = 4 workers.
			t.Logf("Sync calls per second, 1 peer, 4 workers: %d", perSec)
			if perSec < 1000 {
				t.Logf("no spin: under 1000 calls/s")
			} else {
				t.Logf("SPIN: retries with no backoff (each real call = new stream + remote SubscribeBin iterator)")
			}
		})
	}
}
