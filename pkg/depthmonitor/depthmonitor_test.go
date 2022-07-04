// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package depthmonitor_test

import (
	"io"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/depthmonitor"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
)

func newTestSvc(
	t depthmonitor.Topology,
	s depthmonitor.SyncReporter,
	r depthmonitor.ReserveReporter,
	st storage.StateStorer,
	warmupTime time.Duration,
) *depthmonitor.Service {

	var topo depthmonitor.Topology = &mockTopology{}
	if t != nil {
		topo = t
	}

	var syncer depthmonitor.SyncReporter = &mockSyncReporter{}
	if s != nil {
		syncer = s
	}

	var reserve depthmonitor.ReserveReporter = &mockReserveReporter{}
	if r != nil {
		reserve = r
	}

	var storer storage.StateStorer
	if st != nil {
		storer = st
	} else {
		storer = mock.NewStateStore()
	}

	return depthmonitor.New(topo, syncer, reserve, storer, logging.New(io.Discard, 5), warmupTime)
}

func TestDepthMonitorService(t *testing.T) {

	waitForDepth := func(t *testing.T, svc *depthmonitor.Service, depth uint8, timeout time.Duration) {
		t.Helper()
		start := time.Now()
		for {
			if time.Since(start) >= timeout {
				t.Fatalf("timed out waiting for depth expected %d found %d", depth, svc.StorageDepth())
			}
			if svc.StorageDepth() != depth {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			break
		}
	}

	t.Run("stop service within warmup time", func(t *testing.T) {
		svc := newTestSvc(nil, nil, nil, nil, time.Second)
		err := svc.Close()
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("start with neighborhood depth", func(t *testing.T) {
		topo := &mockTopology{connDepth: 3}
		svc := newTestSvc(topo, nil, nil, nil, 100*time.Millisecond)
		time.Sleep(200 * time.Millisecond)
		waitForDepth(t, svc, 3, time.Second)
		err := svc.Close()
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("start with stored depth", func(t *testing.T) {
		topo := &mockTopology{connDepth: 3}

		st := mock.NewStateStore()
		err := st.Put(depthmonitor.DepthKey, uint8(5))
		if err != nil {
			t.Fatal(err)
		}

		svc := newTestSvc(topo, nil, nil, st, 100*time.Millisecond)
		waitForDepth(t, svc, 5, time.Second)
		err = svc.Close()
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("depth decrease due to under utilization", func(t *testing.T) {
		topo := &mockTopology{connDepth: 3}
		// >50% utilized reserve
		reserve := &mockReserveReporter{size: 26000, capacity: 50000}
		// rate not enough to make reserve half full
		// 1 * adaptationWindowSize (2*60*60) = 7200 chunks for 2 hrs
		syncer := &mockSyncReporter{rate: 1}
		st := mock.NewStateStore()

		defer func(w time.Duration) {
			*depthmonitor.ManageWait = w
		}(*depthmonitor.ManageWait)

		*depthmonitor.ManageWait = 200 * time.Millisecond

		svc := newTestSvc(topo, syncer, reserve, st, 100*time.Millisecond)

		waitForDepth(t, svc, 3, time.Second)
		// simulate huge eviction to trigger manage worker
		reserve.setSize(1000)

		waitForDepth(t, svc, 1, time.Second)
		if topo.getStorageDepth() != 1 {
			t.Fatal("topology depth not updated")
		}
		err := svc.Close()
		if err != nil {
			t.Fatal(err)
		}

		// ensure new depth is stored on close
		var storedDepth uint8
		err = st.Get(depthmonitor.DepthKey, &storedDepth)
		if err != nil {
			t.Fatal(err)
		}
		if storedDepth != 1 {
			t.Fatal("incorrect depth stored on shutdown")
		}
	})

	t.Run("depth doesnt change due to high pull rate", func(t *testing.T) {
		topo := &mockTopology{connDepth: 3}
		// under utilized reserve
		reserve := &mockReserveReporter{size: 10000, capacity: 50000}
		// rate very high to ensure reserve will be half full
		// 10 * adaptationWindowSize (2*60*60) = 72000 chunks for 2 hrs
		syncer := &mockSyncReporter{rate: 10}
		st := mock.NewStateStore()

		defer func(w time.Duration) {
			*depthmonitor.ManageWait = w
		}(*depthmonitor.ManageWait)

		*depthmonitor.ManageWait = 200 * time.Millisecond

		svc := newTestSvc(topo, syncer, reserve, st, 100*time.Millisecond)

		time.Sleep(2 * time.Second)
		// ensure that after few cycles of the adaptation period, the depth hasnt
		// changed
		if svc.StorageDepth() != 3 {
			t.Fatal("found drop in depth")
		}
		err := svc.Close()
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("depth doesnt change for utilized reserve", func(t *testing.T) {
		topo := &mockTopology{connDepth: 3}
		// >50% utilized reserve
		reserve := &mockReserveReporter{size: 25001, capacity: 50000}
		// no incoming chunks
		syncer := &mockSyncReporter{rate: 0}
		st := mock.NewStateStore()

		defer func(w time.Duration) {
			*depthmonitor.ManageWait = w
		}(*depthmonitor.ManageWait)

		*depthmonitor.ManageWait = 200 * time.Millisecond

		svc := newTestSvc(topo, syncer, reserve, st, 100*time.Millisecond)

		time.Sleep(2 * time.Second)
		// ensure the depth hasnt changed
		if svc.StorageDepth() != 3 {
			t.Fatal("found drop in depth")
		}
		err := svc.Close()
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("radius setter handler", func(t *testing.T) {
		topo := &mockTopology{connDepth: 3}
		// >50% utilized reserve
		reserve := &mockReserveReporter{size: 25001, capacity: 50000}
		st := mock.NewStateStore()

		defer func(w time.Duration) {
			*depthmonitor.ManageWait = w
		}(*depthmonitor.ManageWait)

		*depthmonitor.ManageWait = 200 * time.Millisecond

		svc := newTestSvc(topo, nil, reserve, st, 100*time.Millisecond)

		time.Sleep(200 * time.Millisecond)
		if svc.StorageDepth() != 3 {
			t.Fatal("incorrect initial depth")
		}

		svc.SetStorageRadius(5)
		if svc.StorageDepth() != 5 {
			t.Fatalf("depth expected 5 found %d", svc.StorageDepth())
		}
		if topo.getStorageDepth() != 5 {
			t.Fatalf("topo depth expected 5 found %d", topo.getStorageDepth())
		}

		err := svc.Close()
		if err != nil {
			t.Fatal(err)
		}
	})
}

type mockTopology struct {
	sync.Mutex
	connDepth    uint8
	storageDepth uint8
}

func (m *mockTopology) NeighborhoodDepth() uint8 {
	return m.connDepth
}

func (m *mockTopology) SetStorageDepth(newDepth uint8) {
	m.Lock()
	defer m.Unlock()
	m.storageDepth = newDepth
}

func (m *mockTopology) getStorageDepth() uint8 {
	m.Lock()
	defer m.Unlock()
	return m.storageDepth
}

type mockSyncReporter struct {
	rate float64
}

func (m *mockSyncReporter) Rate() float64 {
	return m.rate
}

type mockReserveReporter struct {
	sync.Mutex
	capacity uint64
	size     uint64
}

func (m *mockReserveReporter) Size() (uint64, error) {
	m.Lock()
	defer m.Unlock()
	return m.size, nil
}

func (m *mockReserveReporter) setSize(sz uint64) {
	m.Lock()
	defer m.Unlock()
	m.size = sz
}

func (m *mockReserveReporter) Capacity() uint64 {
	return m.capacity
}
