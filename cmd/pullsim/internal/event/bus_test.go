// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"encoding/json"
	"testing"
	"time"
)

type stubProvider struct{ snap Snapshot }

func (s stubProvider) Snapshot() Snapshot { return s.snap }

func decode(t *testing.T, frame []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(frame, &m); err != nil {
		t.Fatalf("bad frame: %v", err)
	}
	return m
}

func TestBus_HelloOnSubscribe(t *testing.T) {
	t.Parallel()

	b := NewBus(stubProvider{snap: Snapshot{Nodes: []NodeSnap{{Index: 0}}}})
	b.Start()
	defer b.Close()

	b.Publish(Config{Nodes: 3, Topology: "full"})
	// give the drainer a moment to store the config
	waitBus(t, func() bool { b.mu.Lock(); defer b.mu.Unlock(); return b.lastConfig.Nodes == 3 })

	ch, unsub := b.Subscribe()
	defer unsub()

	frame := <-ch
	m := decode(t, frame)
	if m["t"] != KindHello {
		t.Fatalf("expected hello, got %v", m["t"])
	}
	cfg := m["config"].(map[string]any)
	if cfg["nodes"].(float64) != 3 {
		t.Fatalf("expected config nodes 3, got %v", cfg["nodes"])
	}
}

func TestBus_SnapshotFoldsEdgeState(t *testing.T) {
	t.Parallel()

	b := NewBus(stubProvider{})
	b.Start()
	defer b.Close()

	ch, unsub := b.Subscribe()
	defer unsub()
	<-ch // hello

	// A sync round on directed edge 0->1: Get, Offer(2), Want, Delivery x2.
	b.Publish(StreamLC{Client: 0, Server: 1, Stream: "pullsync", StreamID: 7, Open: true})
	b.Publish(Msg{Client: 0, Server: 1, Stream: "pullsync", StreamID: 7, Dir: "c2s", Type: "Get", Bin: 3})
	b.Publish(Msg{Client: 0, Server: 1, Stream: "pullsync", StreamID: 7, Dir: "s2c", Type: "Offer", N: 2})
	b.Publish(Msg{Client: 0, Server: 1, Stream: "pullsync", StreamID: 7, Dir: "c2s", Type: "Want", N: 2})
	b.Publish(Msg{Client: 0, Server: 1, Stream: "pullsync", StreamID: 7, Dir: "s2c", Type: "Delivery", N: 1})

	var edgeDir []any
	waitBus(t, func() bool {
		frame := drainLatestSnap(ch)
		if frame == nil {
			return false
		}
		m := decode(t, frame)
		ed, _ := m["edgeDir"].([]any)
		edgeDir = ed
		return len(ed) == 1
	})

	e := edgeDir[0].(map[string]any)
	if e["from"].(float64) != 0 || e["to"].(float64) != 1 {
		t.Fatalf("wrong directed edge: %v", e)
	}
	if e["state"] != EdgeDelivering {
		t.Fatalf("expected delivering state, got %v", e["state"])
	}
}

func TestBus_PublishNeverBlocks(t *testing.T) {
	t.Parallel()

	b := NewBus(stubProvider{})
	// Not started: drainer never consumes, so the channel fills and drops.
	for i := 0; i < publishBuffer*2; i++ {
		b.Publish(Radius{Node: 0, Radius: 1})
	}
	if b.Dropped() == 0 {
		t.Fatal("expected some dropped events when channel is full")
	}
}

func waitBus(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("bus condition not met in time")
}

// drainLatestSnap returns the most recent snapshot frame available without
// blocking, or nil.
func drainLatestSnap(ch <-chan []byte) []byte {
	var latest []byte
	for {
		select {
		case f := <-ch:
			var m map[string]any
			if json.Unmarshal(f, &m) == nil && m["t"] == KindSnap {
				latest = f
			}
		default:
			return latest
		}
	}
}

func TestBus_InjectFrameCarriesTraced(t *testing.T) {
	t.Parallel()

	b := NewBus(stubProvider{})
	b.Start()
	defer b.Close()

	ch, unsub := b.Subscribe()
	defer unsub()
	<-ch // discard the hello frame

	b.Publish(Inject{Node: 2, Count: 1, Traced: true})
	b.Publish(Inject{Node: 3, Count: 10, Traced: false})

	for _, want := range []struct {
		node   float64
		traced bool
	}{{2, true}, {3, false}} {
		select {
		case frame := <-ch:
			m := decode(t, frame)
			if m["t"] != KindInject {
				t.Fatalf("expected inject frame, got %v", m["t"])
			}
			if m["node"].(float64) != want.node {
				t.Fatalf("expected node %v, got %v", want.node, m["node"])
			}
			got, ok := m["traced"]
			if !ok {
				t.Fatal("inject frame has no traced field")
			}
			if got.(bool) != want.traced {
				t.Fatalf("node %v: expected traced %v, got %v", want.node, want.traced, got)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for inject frame")
		}
	}
}
