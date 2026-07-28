// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sim_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/cmd/pullsim/internal/sim"
)

// clock is a manually advanced time source.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }
func newClock() *clock                   { return &clock{t: time.Unix(1, 0)} }
func keys(ks ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(ks))
	for _, k := range ks {
		out[k] = struct{}{}
	}
	return out
}

// TestHealSettlesOnQuiescenceWithResidual: an episode where only part of the
// deficit is ever recovered must still settle and report the residue, rather
// than waiting forever for chunks no peer can serve.
func TestHealSettlesOnQuiescenceWithResidual(t *testing.T) {
	t.Parallel()

	c := newClock()
	tr := sim.NewHealTracker(time.Second, c.now)

	id := tr.Open(4, 3, map[int]map[string]struct{}{
		0: keys("a", "b"),
		1: keys("c"),
		2: nil, // nothing owed: must not appear in the report at all
	})

	c.advance(200 * time.Millisecond)
	tr.ObservePut(0, "a", sim.PutSourceSync)
	// An inject is the operator putting the chunk there, not the network
	// healing itself.
	tr.ObservePut(1, "c", sim.PutSourceInject)

	if got := tr.Sweep(); len(got) != 0 {
		t.Fatalf("got %d settled episodes before the quiescence window, want 0", len(got))
	}

	c.advance(2 * time.Second)
	settled := tr.Sweep()
	if len(settled) != 1 {
		t.Fatalf("got %d settled episodes, want 1", len(settled))
	}
	h := settled[0]
	if h.ID != id {
		t.Errorf("got ID %d, want %d", h.ID, id)
	}
	if h.FromRadius != 4 || h.ToRadius != 3 {
		t.Errorf("got radius transition %d->%d, want 4->3", h.FromRadius, h.ToRadius)
	}
	if h.Total != 3 || h.Healed != 1 || h.Remaining != 2 {
		t.Errorf("got total/healed/remaining %d/%d/%d, want 3/1/2", h.Total, h.Healed, h.Remaining)
	}
	if !h.Settled {
		t.Error("episode not settled")
	}
	if h.HealSpanMs != 200 {
		t.Errorf("got HealSpanMs %d, want 200 (measured to the last heal, not to settle time)", h.HealSpanMs)
	}
	want := []sim.NodeHeal{{Node: 0, Total: 2, Healed: 1}, {Node: 1, Total: 1, Healed: 0}}
	if !reflect.DeepEqual(h.PerNode, want) {
		t.Errorf("got per-node %v, want %v", h.PerNode, want)
	}

	// A settled episode is only reported once.
	c.advance(time.Hour)
	if got := tr.Sweep(); len(got) != 0 {
		t.Errorf("got %d episodes on a second sweep, want 0", len(got))
	}
}

// TestHealSettlesImmediatelyAtZero: the last arriving chunk closes the episode
// without burning a whole quiescence window.
func TestHealSettlesImmediatelyAtZero(t *testing.T) {
	t.Parallel()

	c := newClock()
	tr := sim.NewHealTracker(time.Hour, c.now) // a window no test would wait out

	id := tr.Open(5, 4, map[int]map[string]struct{}{0: keys("a", "b")})
	c.advance(50 * time.Millisecond)
	tr.ObservePut(0, "a", sim.PutSourceSync)
	c.advance(50 * time.Millisecond)
	tr.ObservePut(0, "b", sim.PutSourceSync)

	h, ok := tr.Get(id)
	if !ok {
		t.Fatal("episode not retained")
	}
	if !h.Settled {
		t.Fatal("episode with zero remaining did not settle immediately")
	}
	if h.Remaining != 0 || h.Healed != 2 {
		t.Errorf("got healed/remaining %d/%d, want 2/0", h.Healed, h.Remaining)
	}
	if h.HealSpanMs != 100 {
		t.Errorf("got HealSpanMs %d, want 100", h.HealSpanMs)
	}
	if got := tr.Sweep(); len(got) != 1 {
		t.Errorf("got %d episodes from the sweep, want the settled one to be published once", len(got))
	}
}

// TestHealEmptyEpisodeSettlesAtOnce: a radius decrease that opens no deficit is
// a completed episode, not a pending one.
func TestHealEmptyEpisodeSettlesAtOnce(t *testing.T) {
	t.Parallel()

	c := newClock()
	tr := sim.NewHealTracker(time.Hour, c.now)
	id := tr.Open(3, 2, map[int]map[string]struct{}{0: nil, 1: {}})

	h, ok := tr.Get(id)
	if !ok {
		t.Fatal("episode not retained")
	}
	if !h.Settled || h.Total != 0 || h.Remaining != 0 {
		t.Errorf("got %+v, want a settled empty episode", h)
	}
	if len(h.PerNode) != 0 {
		t.Errorf("got %d per-node rows, want 0", len(h.PerNode))
	}
}

// TestHealIgnoresUnrelatedPuts: a put for a chunk the node never owed, or for a
// node with no deficit, must not inflate the healed count.
func TestHealIgnoresUnrelatedPuts(t *testing.T) {
	t.Parallel()

	c := newClock()
	tr := sim.NewHealTracker(time.Second, c.now)
	tr.Open(4, 3, map[int]map[string]struct{}{0: keys("a")})

	tr.ObservePut(0, "zzz", sim.PutSourceSync)
	tr.ObservePut(1, "a", sim.PutSourceSync)

	c.advance(2 * time.Second)
	settled := tr.Sweep()
	if len(settled) != 1 {
		t.Fatalf("got %d settled episodes, want 1", len(settled))
	}
	if settled[0].Healed != 0 || settled[0].Remaining != 1 {
		t.Errorf("got healed/remaining %d/%d, want 0/1", settled[0].Healed, settled[0].Remaining)
	}
}

// TestHealEpisodeOpensOnlyOnDecrease exercises the SetRadius trigger through
// the Network's public surface.
func TestHealEpisodeOpensOnlyOnDecrease(t *testing.T) {
	t.Parallel()

	n := buildNet(t, sim.Config{
		Nodes: 4, Bins: 8, Topology: sim.TopologyFull, Radius: 4, Seed: 31,
	})

	n.SetRadius(4) // no-op
	if got := len(n.Heals()); got != 0 {
		t.Fatalf("got %d heal episodes after a no-op radius set, want 0", got)
	}
	n.SetRadius(6) // increase
	if got := len(n.Heals()); got != 0 {
		t.Fatalf("got %d heal episodes after a radius increase, want 0", got)
	}

	n.SetRadius(2) // decrease
	heals := n.Heals()
	if len(heals) != 1 {
		t.Fatalf("got %d heal episodes after a radius decrease, want 1", len(heals))
	}
	if heals[0].FromRadius != 6 || heals[0].ToRadius != 2 {
		t.Errorf("got radius transition %d->%d, want 6->2", heals[0].FromRadius, heals[0].ToRadius)
	}
	if _, ok := n.Heal(heals[0].ID); !ok {
		t.Errorf("Heal(%d) not found", heals[0].ID)
	}
	if _, ok := n.Heal(heals[0].ID + 1000); ok {
		t.Error("Heal returned an unknown episode")
	}
}
