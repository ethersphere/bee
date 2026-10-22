// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestCreditBook checks the per-peer fetch credit's book (T4.7 part 4): what
// a new overlay starts with, what asking and delivering cost and give back,
// the probe of a peer with no credit left, the node-wide pool of initial
// credit, and the bound on the overlays it keeps.
func TestCreditBook(t *testing.T) {
	t.Parallel()
	start := time.Unix(1_000_000, 0)

	t.Run("new overlay", func(t *testing.T) {
		t.Parallel()
		b := rangesync.NewCreditBook(rangesync.MaxCreditPeers, start)
		p := swarm.RandAddress(t)
		if c, known := b.Credit(p); c != 0 || known {
			t.Fatalf("unknown peer: credit %d known %v, want 0 false", c, known)
		}
		if got := b.Take(p, 10, start); got != 10 {
			t.Fatalf("took %d, want 10", got)
		}
		if c, known := b.Credit(p); c != rangesync.CreditInitial-10 || !known {
			t.Fatalf("credit %d known %v, want %d true", c, known, rangesync.CreditInitial-10)
		}
		// more than is left: what is left
		if got := b.Take(p, rangesync.CreditInitial, start); got != rangesync.CreditInitial-10 {
			t.Fatalf("took %d, want %d", got, rangesync.CreditInitial-10)
		}
		if got := b.Take(p, 1, start); got != 0 {
			t.Fatalf("took %d with no credit left, want 0", got)
		}
	})

	t.Run("delivered", func(t *testing.T) {
		t.Parallel()
		b := rangesync.NewCreditBook(rangesync.MaxCreditPeers, start)
		p := swarm.RandAddress(t)
		n := b.Take(p, rangesync.CreditInitial, start)
		// every chunk stored gives back its unit and one more
		b.Give(p, n*rangesync.CreditStoredRefund)
		if c, _ := b.Credit(p); c != 2*rangesync.CreditInitial {
			t.Fatalf("credit %d, want %d", c, 2*rangesync.CreditInitial)
		}
		// never past creditMax
		b.Give(p, 10*rangesync.CreditMax)
		if c, _ := b.Credit(p); c != rangesync.CreditMax {
			t.Fatalf("credit %d, want %d", c, rangesync.CreditMax)
		}
		// a peer the book forgot gets nothing back
		q := swarm.RandAddress(t)
		b.Give(q, 10)
		if c, known := b.Credit(q); c != 0 || known {
			t.Fatalf("given to an unknown peer: credit %d known %v", c, known)
		}
	})

	t.Run("probe", func(t *testing.T) {
		t.Parallel()
		b := rangesync.NewCreditBook(rangesync.MaxCreditPeers, start)
		p := swarm.RandAddress(t)
		b.Take(p, rangesync.CreditInitial, start)
		now := start
		if got := b.Take(p, 10, now); got != 0 {
			t.Fatalf("took %d before the first probe, want 0", got)
		}
		now = now.Add(rangesync.CreditProbeInterval)
		if got := b.Take(p, 10, now); got != 1 {
			t.Fatalf("probe took %d, want 1", got)
		}
		if got := b.Take(p, 10, now.Add(rangesync.CreditProbeInterval-time.Nanosecond)); got != 0 {
			t.Fatalf("took %d before the next probe, want 0", got)
		}
		// an unspent probe stays: a Want that did not run gives it back
		now = now.Add(rangesync.CreditProbeInterval)
		if got := b.Take(p, 10, now); got != 1 {
			t.Fatalf("probe took %d, want 1", got)
		}
		b.Give(p, 1)
		if got := b.Take(p, 10, now); got != 1 {
			t.Fatalf("took %d of the probe given back, want 1", got)
		}
		// a probe delivered doubles: one chunk stored gives two
		b.Give(p, rangesync.CreditStoredRefund)
		if got := b.Take(p, 10, now); got != 2 {
			t.Fatalf("took %d after the probe was delivered, want 2", got)
		}
		// at most one probe per interval over a long gap
		now = now.Add(10 * rangesync.CreditProbeInterval)
		if got := b.Take(p, 10, now); got != 1 {
			t.Fatalf("probe after a long gap took %d, want 1", got)
		}
		if got := b.Take(p, 10, now); got != 0 {
			t.Fatalf("second probe at once took %d, want 0", got)
		}
	})

	t.Run("pool", func(t *testing.T) {
		t.Parallel()
		b := rangesync.NewCreditBook(rangesync.MaxCreditPeers, start)
		newcomers := rangesync.CreditPoolBurst / rangesync.CreditInitial
		for i := range newcomers {
			if got := b.Take(swarm.RandAddress(t), rangesync.CreditInitial, start); got != rangesync.CreditInitial {
				t.Fatalf("newcomer %d took %d, want %d", i, got, rangesync.CreditInitial)
			}
		}
		// the pool is drained: a new overlay starts with nothing, and its
		// first probe comes one interval after it was first asked for
		late := swarm.RandAddress(t)
		if got := b.Take(late, rangesync.CreditInitial, start); got != 0 {
			t.Fatalf("newcomer past the pool took %d, want 0", got)
		}
		if got := b.Take(late, 1, start.Add(rangesync.CreditProbeInterval)); got != 1 {
			t.Fatalf("newcomer's probe took %d, want 1", got)
		}
		// the pool regains one initial credit per interval, and gives part
		// of one
		half := start.Add(rangesync.CreditPoolInterval / 2)
		if got := b.Take(swarm.RandAddress(t), rangesync.CreditInitial, half); got != rangesync.CreditInitial/2 {
			t.Fatalf("newcomer at half the pool interval took %d, want %d", got, rangesync.CreditInitial/2)
		}
		later := half.Add(rangesync.CreditPoolInterval)
		if got := b.Take(swarm.RandAddress(t), rangesync.CreditInitial, later); got != rangesync.CreditInitial {
			t.Fatalf("newcomer one pool interval later took %d, want %d", got, rangesync.CreditInitial)
		}
		// a known peer draws nothing from the pool, whatever it spent: it
		// gets its probe
		if got := b.Take(late, rangesync.CreditInitial, later); got != 1 {
			t.Fatalf("known peer with the pool refilled took %d, want its probe", got)
		}
	})

	t.Run("evicted", func(t *testing.T) {
		t.Parallel()
		const capacity = 4
		b := rangesync.NewCreditBook(capacity, start)
		// fill fills the book with capacity overlays asked after the others,
		// each spending all its credit, so in debt
		fill := func(now time.Time) {
			for range capacity {
				b.Take(swarm.RandAddress(t), rangesync.CreditInitial, now)
			}
		}
		sybil := swarm.RandAddress(t)
		b.Take(sybil, rangesync.CreditInitial, start)
		fill(start) // forgets it, the least recently asked, in debt
		if _, known := b.Credit(sybil); known {
			t.Fatal("least recently asked overlay kept past the capacity")
		}
		if peers, debts := b.Len(); peers != capacity || debts != 1 {
			t.Fatalf("book keeps %d overlays and %d debts, want %d and 1", peers, debts, capacity)
		}
		// asked again, it resumes its debt, although the pool has credit,
		// and its probe clock starts over
		now := start.Add(rangesync.CreditProbeInterval)
		if got := b.Take(sybil, 1, now); got != 0 {
			t.Fatalf("forgotten debtor back took %d, want 0", got)
		}
		if got := b.Take(sybil, 1, now.Add(rangesync.CreditProbeInterval-time.Nanosecond)); got != 0 {
			t.Fatalf("forgotten debtor back took %d before its probe, want 0", got)
		}
		if got := b.Take(sybil, 1, now.Add(rangesync.CreditProbeInterval)); got != 1 {
			t.Fatalf("forgotten debtor's probe took %d, want 1", got)
		}

		// an overlay forgotten with at least the initial credit is not
		// remembered: back, it gets at most what it had
		honest := swarm.RandAddress(t)
		n := b.Take(honest, rangesync.CreditInitial, now)
		b.Give(honest, n*rangesync.CreditStoredRefund)
		fill(now)
		if _, known := b.Credit(honest); known {
			t.Fatal("least recently asked overlay kept past the capacity")
		}
		if got := b.Take(honest, rangesync.CreditMax, now); got != rangesync.CreditInitial {
			t.Fatalf("forgotten creditor back took %d, want %d", got, rangesync.CreditInitial)
		}

		// the debts are bounded: once capacity more overlays were forgotten
		// in debt after it, the oldest debt is forgotten, and that overlay
		// back is a new one
		peers, debts := b.Len()
		if peers > capacity || debts > capacity {
			t.Fatalf("book keeps %d overlays and %d debts, over %d", peers, debts, capacity)
		}
		fill(now)
		fill(now)
		if got := b.Take(sybil, rangesync.CreditInitial, now); got != rangesync.CreditInitial {
			t.Fatalf("overlay whose debt was forgotten took %d, want the pool's %d", got, rangesync.CreditInitial)
		}
	})
}
