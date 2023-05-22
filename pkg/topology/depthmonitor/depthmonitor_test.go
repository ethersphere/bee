// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package depthmonitor_test

import (
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	mockbatchstore "github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	"github.com/ethersphere/bee/pkg/spinlock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/topology/depthmonitor"
)

const depthMonitorWakeUpInterval = 10 * time.Millisecond

func newTestSvc(
	t depthmonitor.Topology,
	s depthmonitor.SyncReporter,
	r depthmonitor.Reserve,
	st storage.StateStorer,
	bs postage.Storer,
	warmupTime time.Duration,
	wakeupInterval time.Duration,
	freshNode bool,
) *depthmonitor.Service {

	var topo depthmonitor.Topology = &mockTopology{}
	if t != nil {
		topo = t
	}

	var syncer depthmonitor.SyncReporter = &mockSyncReporter{}
	if s != nil {
		syncer = s
	}

	var reserve depthmonitor.Reserve = &mockReserve{}
	if r != nil {
		reserve = r
	}

	batchStore := postage.Storer(mockbatchstore.New())
	if bs != nil {
		batchStore = bs
	}

	return depthmonitor.New(topo, syncer, reserve, batchStore, log.Noop, warmupTime, wakeupInterval, freshNode)
}

func TestDepthMonitorService_FLAKY(t *testing.T) {
	t.Parallel()

	waitForDepth := func(t *testing.T, svc *depthmonitor.Service, depth uint8) {
		t.Helper()

		err := spinlock.Wait(time.Second*3, func() bool {
			return svc.StorageDepth() == depth
		})
		if err != nil {
			t.Fatalf("timed out waiting for depth expected %d found %d", depth, svc.StorageDepth())
		}
	}

	t.Run("stop service within warmup time", func(t *testing.T) {
		t.Parallel()

		svc := newTestSvc(nil, nil, nil, nil, nil, time.Second, depthmonitor.DefaultWakeupInterval, true)
		err := svc.Close()
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("start with radius", func(t *testing.T) {
		t.Parallel()

		bs := mockbatchstore.New(mockbatchstore.WithReserveState(&postage.ReserveState{Radius: 3}))
		svc := newTestSvc(nil, nil, nil, nil, bs, 0, depthmonitor.DefaultWakeupInterval, true)
		waitForDepth(t, svc, 3)
		err := svc.Close()
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("depth decrease due to under utilization", func(t *testing.T) {
		t.Parallel()

		topo := &mockTopology{peers: 1}
		// >40% utilized reserve
		reserve := &mockReserve{size: 25001, capacity: 50000}

		bs := mockbatchstore.New(mockbatchstore.WithReserveState(&postage.ReserveState{Radius: 3}))

		svc := newTestSvc(topo, nil, reserve, nil, bs, 0, depthMonitorWakeUpInterval, true)

		waitForDepth(t, svc, 3)
		// simulate huge eviction to trigger manage worker
		reserve.setSize(1000)

		waitForDepth(t, svc, 0)
		err := svc.Close()
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("depth doesnt change due to non-zero pull rate", func(t *testing.T) {
		t.Parallel()

		// under utilized reserve
		reserve := &mockReserve{size: 10000, capacity: 50000}
		bs := mockbatchstore.New(mockbatchstore.WithReserveState(&postage.ReserveState{Radius: 3}))
		syncer := &mockSyncReporter{rate: 10}

		svc := newTestSvc(nil, syncer, reserve, nil, bs, 0, depthMonitorWakeUpInterval, true)

		time.Sleep(time.Second)
		// ensure that after few cycles of the adaptation period, the depth hasn't changed
		if svc.StorageDepth() != 3 {
			t.Fatal("found drop in depth")
		}
		err := svc.Close()
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("depth doesnt change for utilized reserve", func(t *testing.T) {
		t.Parallel()

		// >40% utilized reserve
		reserve := &mockReserve{size: 20001, capacity: 50000}
		bs := mockbatchstore.New(mockbatchstore.WithReserveState(&postage.ReserveState{Radius: 3}))

		svc := newTestSvc(nil, nil, reserve, nil, bs, 0, depthMonitorWakeUpInterval, true)

		time.Sleep(time.Second)
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
		t.Parallel()

		topo := &mockTopology{connDepth: 3}
		bs := mockbatchstore.New(mockbatchstore.WithReserveState(&postage.ReserveState{Radius: 3}))
		// >40% utilized reserve
		reserve := &mockReserve{size: 20001, capacity: 50000}

		svc := newTestSvc(topo, nil, reserve, nil, bs, 0, depthMonitorWakeUpInterval, true)

		waitForDepth(t, svc, 3)

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
	peers        int
}

func (m *mockTopology) SetStorageRadius(newDepth uint8) {
	m.Lock()
	defer m.Unlock()
	m.storageDepth = newDepth
}

func (m *mockTopology) PeersCount(topology.Select) int {
	return m.peers
}

func (m *mockTopology) getStorageDepth() uint8 {
	m.Lock()
	defer m.Unlock()
	return m.storageDepth
}

type mockSyncReporter struct {
	rate float64
}

func (m *mockSyncReporter) SyncRate() float64 {
	return m.rate
}

type mockReserve struct {
	sync.Mutex
	capacity uint64
	size     uint64
}

func (m *mockReserve) ComputeReserveSize(uint8) (uint64, error) {
	m.Lock()
	defer m.Unlock()
	return m.size, nil
}

func (m *mockReserve) ReserveCapacity() uint64 {
	return m.capacity
}

func (m *mockReserve) setSize(sz uint64) {
	m.Lock()
	defer m.Unlock()
	m.size = sz
}
