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

// waitFrame reads frames until one of the given kind arrives.
func waitFrame(t *testing.T, ch <-chan []byte, kind string) []byte {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case frame := <-ch:
			if decode(t, frame)["t"] == kind {
				return frame
			}
		case <-deadline:
			t.Fatalf("no %q frame within timeout", kind)
		}
	}
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

func TestBus_BroadcastsBatchFrame(t *testing.T) {
	b := NewBus(stubProvider{})
	b.Start()
	defer b.Close()

	ch, cancel := b.Subscribe()
	defer cancel()
	<-ch // hello

	b.Publish(Batch{BatchSnap: BatchSnap{
		ID: 7, Origin: 3, Chunks: 10, Replicas: 42, NodesReached: 9,
		Settled: true, LateReplicas: 5, SpanMs: 1234, InjectMs: 0, TailMs: 1234,
		PerDeliveryP50Ms: 300, PerDeliveryP95Ms: 900, PerDeliveryMaxMs: 1100,
	}})

	frame := waitFrame(t, ch, KindBatch)
	m := decode(t, frame)
	batch, ok := m["batch"].(map[string]any)
	if !ok {
		t.Fatalf("frame has no batch object: %v", m)
	}
	if batch["id"] != float64(7) {
		t.Errorf("got id %v, want 7", batch["id"])
	}
	if batch["spanMs"] != float64(1234) {
		t.Errorf("got spanMs %v, want 1234", batch["spanMs"])
	}
	if batch["settled"] != true {
		t.Errorf("got settled %v, want true", batch["settled"])
	}
	// I1: truncation evidence has to reach the browser.
	if batch["lateReplicas"] != float64(5) {
		t.Errorf("got lateReplicas %v, want 5", batch["lateReplicas"])
	}
	// I2: the percentiles are a per-delivery distribution, and are named so.
	for k, want := range map[string]float64{
		"perDeliveryP50Ms": 300, "perDeliveryP95Ms": 900, "perDeliveryMaxMs": 1100,
	} {
		if batch[k] != want {
			t.Errorf("got %s %v, want %v", k, batch[k], want)
		}
	}
	for _, gone := range []string{"perChunkP50Ms", "perChunkP95Ms", "perChunkMaxMs"} {
		if _, ok := batch[gone]; ok {
			t.Errorf("stale field %s still on the wire", gone)
		}
	}
}

// I6: the settle window must reach the browser, or a rebuild silently resets a
// non-default -settle back to 3s.
func TestBus_ConfigCarriesSettleWindow(t *testing.T) {
	b := NewBus(stubProvider{})
	b.Start()
	defer b.Close()

	ch, cancel := b.Subscribe()
	defer cancel()
	<-ch // hello

	b.Publish(Config{Nodes: 4, Topology: "full", SettleAfterMs: 10000})

	m := decode(t, waitFrame(t, ch, KindConfig))
	cfg, ok := m["config"].(map[string]any)
	if !ok {
		t.Fatalf("frame has no config object: %v", m)
	}
	if cfg["settleAfterMs"] != float64(10000) {
		t.Errorf("got settleAfterMs %v, want 10000", cfg["settleAfterMs"])
	}
}

func TestBus_BatchFramesAreNotRateLimited(t *testing.T) {
	b := NewBus(stubProvider{})
	b.Start()
	defer b.Close()

	ch, cancel := b.Subscribe()
	defer cancel()
	<-ch // hello

	// Well past maxDiscretePerSec; every batch frame must still be delivered.
	const n = maxDiscretePerSec + 20
	for i := 0; i < n; i++ {
		b.Publish(Batch{BatchSnap: BatchSnap{ID: i + 1}})
	}
	seen := 0
	deadline := time.After(3 * time.Second)
	for seen < n {
		select {
		case frame := <-ch:
			if decode(t, frame)["t"] == KindBatch {
				seen++
			}
		case <-deadline:
			t.Fatalf("got %d batch frames, want %d", seen, n)
		}
	}
}

func TestBus_SnapshotCarriesBatches(t *testing.T) {
	b := NewBus(stubProvider{snap: Snapshot{
		Batches: []BatchSnap{{ID: 1, Chunks: 5, SpanMs: 900}},
	}})
	b.Start()
	defer b.Close()

	ch, cancel := b.Subscribe()
	defer cancel()

	m := decode(t, <-ch) // hello carries a snapshot
	snap, ok := m["snap"].(map[string]any)
	if !ok {
		t.Fatalf("hello has no snap: %v", m)
	}
	batches, ok := snap["batches"].([]any)
	if !ok || len(batches) != 1 {
		t.Fatalf("got batches %v, want one entry", snap["batches"])
	}
	first := batches[0].(map[string]any)
	if first["chunks"] != float64(5) {
		t.Errorf("got chunks %v, want 5", first["chunks"])
	}
}
