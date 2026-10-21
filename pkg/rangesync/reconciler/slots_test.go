// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
)

// acquireAsync starts p.Acquire(ctx) and returns a channel that receives its
// release func, or nil on an error, which goes to errs.
func acquireAsync(ctx context.Context, p reconciler.SlotPool, errs chan<- error) <-chan func() {
	got := make(chan func(), 1)
	go func() {
		release, err := p.Acquire(ctx)
		if err != nil {
			errs <- err
			got <- nil
			return
		}
		got <- release
	}()
	return got
}

// waitUsed waits until p has used slots taken, or fails.
func waitUsed(t *testing.T, p reconciler.SlotPool, used int) {
	t.Helper()
	waitFor(t, "slots taken", func() bool { _, u := p.State(); return u == used })
}

// waitWaiting waits until n callers wait for a slot of p, or fails.
func waitWaiting(t *testing.T, p reconciler.SlotPool, n int) {
	t.Helper()
	waitFor(t, "waiters", func() bool { return p.Waiting() == n })
}

// TestSlotPool: the semaphore of the per-node cap grants slots first come,
// first served; a smaller size takes no slot from a holder and lets no
// waiter in until the slots in use are below it; a larger one lets the
// waiters in, in order; a waiter whose context ends takes no slot.
func TestSlotPool(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p := reconciler.NewSlotPool(2)
	errs := make(chan error, 8)
	r1, err := p.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := p.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	w1 := acquireAsync(ctx, p, errs)
	waitWaiting(t, p, 1)
	w2 := acquireAsync(ctx, p, errs)
	waitWaiting(t, p, 2)
	if len(w1) != 0 || len(w2) != 0 {
		t.Fatal("a waiter got a slot of a full pool")
	}

	// first come, first served
	r1()
	r3 := <-w1
	if r3 == nil {
		t.Fatal(<-errs)
	}
	if len(w2) != 0 {
		t.Fatal("the second waiter got the slot of the first")
	}

	// smaller: the holders keep their slots, the waiter waits until the
	// slots in use are below the new size
	p.Resize(1)
	if size, used := p.State(); size != 1 || used != 2 {
		t.Fatalf("size %d, used %d; want 1, 2", size, used)
	}
	r2()
	if p.Waiting() != 1 || len(w2) != 0 {
		t.Fatal("a waiter got a slot with the pool at its size")
	}
	r3()
	r4 := <-w2
	if r4 == nil {
		t.Fatal(<-errs)
	}

	// larger: the waiters get the new slots in order
	w3 := acquireAsync(ctx, p, errs)
	waitWaiting(t, p, 1)
	w4 := acquireAsync(ctx, p, errs)
	waitWaiting(t, p, 2)
	p.Resize(3)
	r5, r6 := <-w3, <-w4
	if r5 == nil || r6 == nil {
		t.Fatal(<-errs)
	}
	waitUsed(t, p, 3)

	// a waiter whose context ends takes no slot, and the next waiter gets
	// the next slot
	cctx, cancel := context.WithCancelCause(ctx)
	cause := errors.New("canceled waiter")
	w5 := acquireAsync(cctx, p, errs)
	waitWaiting(t, p, 1)
	w6 := acquireAsync(ctx, p, errs)
	waitWaiting(t, p, 2)
	cancel(cause)
	if r := <-w5; r != nil {
		t.Fatal("a canceled waiter got a slot")
	}
	if err := <-errs; !errors.Is(err, cause) {
		t.Fatalf("canceled waiter: %v, want %v", err, cause)
	}
	waitWaiting(t, p, 1)
	r4()
	r7 := <-w6
	if r7 == nil {
		t.Fatal(<-errs)
	}
	r5()
	r6()
	r7()
	waitUsed(t, p, 0)
}

// TestSlotPoolCancelRace: waiters whose contexts end while slots are freed
// and the size changes leave the count of slots in use exact.
func TestSlotPoolCancelRace(t *testing.T) {
	t.Parallel()

	p := reconciler.NewSlotPool(2)
	errs := make(chan error, 200)
	ctx, cancel := context.WithCancel(context.Background())
	waiters := make([]<-chan func(), 0, 100)
	for range cap(waiters) {
		waiters = append(waiters, acquireAsync(ctx, p, errs))
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 50 {
			p.Resize(1 + i%4)
		}
	}()
	time.Sleep(time.Millisecond)
	cancel()
	<-done
	for _, w := range waiters {
		if r := <-w; r != nil {
			r()
		}
	}
	waitUsed(t, p, 0)
	p.Resize(2)
	r, err := p.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r()
}
