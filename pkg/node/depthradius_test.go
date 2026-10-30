// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/depth"
	depthmock "github.com/ethersphere/bee/v2/pkg/depth/mock"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/node"
	"github.com/ethersphere/bee/v2/pkg/storer"
)

const (
	testRoundLength = 20 * time.Millisecond
	// testDeadline is the initial-radius deadline, two round lengths after the first call.
	testDeadline = 2 * testRoundLength
	// testPrompt bounds calls that must not wait for a deadline.
	testPrompt = 200 * time.Millisecond
)

// setAfter sets the provider to depth d in state s after delay, from a timer goroutine.
func setAfter(t *testing.T, p *depthmock.Provider, delay time.Duration, d uint8, s depth.State) {
	t.Helper()
	timer := time.AfterFunc(delay, func() { p.Set(d, s) })
	t.Cleanup(func() { timer.Stop() })
}

func TestInitialRadiusFunc(t *testing.T) {
	t.Parallel()

	noCommitment := func(t *testing.T) func() (uint64, error) {
		t.Helper()
		return func() (uint64, error) {
			t.Error("commitment called")
			return 0, nil
		}
	}
	syncDone := func() bool { return true }

	for _, tc := range []struct {
		name              string
		depth             uint8
		state             depth.State
		doubling, minimum uint8
		want              uint8
	}{
		{name: "known minus doubling", depth: 5, state: depth.StateKnown, doubling: 1, want: 4},
		{name: "known below doubling clamps to zero", depth: 0, state: depth.StateKnown, doubling: 1, want: 0},
		{name: "known below minimum", depth: 3, state: depth.StateKnown, minimum: 4, want: 4},
		{name: "no history", depth: 9, state: depth.StateNoHistory, doubling: 1, minimum: 2, want: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := depthmock.New(depthmock.WithDepth(tc.depth), depthmock.WithState(tc.state))
			f := node.NewInitialRadiusFunc(p, tc.doubling, tc.minimum, testRoundLength, noCommitment(t), syncDone, log.Noop)

			start := time.Now()
			got, err := f(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got radius %d, want %d", got, tc.want)
			}
			if elapsed := time.Since(start); elapsed >= testPrompt {
				t.Fatalf("returned after %v, want before %v", elapsed, testPrompt)
			}
		})
	}

	t.Run("no data then known", func(t *testing.T) {
		t.Parallel()

		p := depthmock.New()
		// syncDone false keeps a late Set from turning into the fallback.
		f := node.NewInitialRadiusFunc(p, 1, 0, testRoundLength, noCommitment(t), func() bool { return false }, log.Noop)
		setAfter(t, p, 5*time.Millisecond, 6, depth.StateKnown)

		start := time.Now()
		got, err := f(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 5 {
			t.Fatalf("got radius %d, want 5", got)
		}
		if elapsed := time.Since(start); elapsed >= testPrompt {
			t.Fatalf("returned after %v, want before %v", elapsed, testPrompt)
		}
	})

	t.Run("no data past the deadline falls back to the ceiling", func(t *testing.T) {
		t.Parallel()

		p := depthmock.New()
		commitment := func() (uint64, error) { return 4 * storer.DefaultReserveCapacity, nil }
		f := node.NewInitialRadiusFunc(p, 1, 0, testRoundLength, commitment, syncDone, log.Noop)

		start := time.Now()
		got, err := f(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Ceiling(4 × capacity) is 2, minus doubling 1.
		if got != 1 {
			t.Fatalf("got radius %d, want 1", got)
		}
		if elapsed := time.Since(start); elapsed < testDeadline {
			t.Fatalf("fallback after %v, want not before %v", elapsed, testDeadline)
		}
	})

	t.Run("fallback waits for batch sync", func(t *testing.T) {
		t.Parallel()

		var syncCalls, syncCallsAtCommitment atomic.Int32
		syncDone := func() bool { return syncCalls.Add(1) >= 2 }
		commitment := func() (uint64, error) {
			syncCallsAtCommitment.Store(syncCalls.Load())
			return 4 * storer.DefaultReserveCapacity, nil
		}
		p := depthmock.New()
		f := node.NewInitialRadiusFunc(p, 0, 0, testRoundLength, commitment, syncDone, log.Noop)

		start := time.Now()
		got, err := f(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 2 {
			t.Fatalf("got radius %d, want 2", got)
		}
		if n := syncCallsAtCommitment.Load(); n != 2 {
			t.Fatalf("commitment read after %d batch sync checks, want 2", n)
		}
		if want, elapsed := testDeadline+testRoundLength, time.Since(start); elapsed < want {
			t.Fatalf("fallback after %v, want not before %v", elapsed, want)
		}
	})

	t.Run("commitment error is retried", func(t *testing.T) {
		t.Parallel()

		var calls atomic.Int32
		commitment := func() (uint64, error) {
			if calls.Add(1) == 1 {
				return 0, errors.New("commitment")
			}
			return 4 * storer.DefaultReserveCapacity, nil
		}
		p := depthmock.New()
		f := node.NewInitialRadiusFunc(p, 0, 0, testRoundLength, commitment, syncDone, log.Noop)

		start := time.Now()
		got, err := f(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Ceiling(4 × capacity) is 2, no doubling.
		if got != 2 {
			t.Fatalf("got radius %d, want 2", got)
		}
		if n := calls.Load(); n != 2 {
			t.Fatalf("commitment read %d times, want 2", n)
		}
		if want, elapsed := testDeadline+testRoundLength, time.Since(start); elapsed < want {
			t.Fatalf("fallback after %v, want not before %v", elapsed, want)
		}
	})

	t.Run("context canceled while waiting", func(t *testing.T) {
		t.Parallel()

		p := depthmock.New()
		// A round of an hour leaves the context as the only way out.
		f := node.NewInitialRadiusFunc(p, 0, 0, time.Hour, noCommitment(t), syncDone, log.Noop)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		timer := time.AfterFunc(5*time.Millisecond, cancel)
		defer timer.Stop()

		if _, err := f(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("got error %v, want %v", err, context.Canceled)
		}
	})
}

func TestLightRadiusFunc(t *testing.T) {
	t.Parallel()

	t.Run("known", func(t *testing.T) {
		t.Parallel()

		p := depthmock.New(depthmock.WithDepth(7), depthmock.WithState(depth.StateKnown))
		f := node.NewLightRadiusFunc(p, time.Hour, time.Now)

		start := time.Now()
		got, err := f(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 7 {
			t.Fatalf("got radius %d, want 7", got)
		}
		if elapsed := time.Since(start); elapsed >= testPrompt {
			t.Fatalf("returned after %v, want before %v", elapsed, testPrompt)
		}
	})

	t.Run("no history", func(t *testing.T) {
		t.Parallel()

		p := depthmock.New(depthmock.WithDepth(7), depthmock.WithState(depth.StateNoHistory))
		f := node.NewLightRadiusFunc(p, time.Hour, time.Now)

		start := time.Now()
		if _, err := f(context.Background()); !errors.Is(err, depth.ErrUnknown) {
			t.Fatalf("got error %v, want %v", err, depth.ErrUnknown)
		}
		if elapsed := time.Since(start); elapsed >= testPrompt {
			t.Fatalf("returned after %v, want before %v", elapsed, testPrompt)
		}
	})

	t.Run("no data then known inside the wait", func(t *testing.T) {
		t.Parallel()

		p := depthmock.New()
		// A round of an hour leaves the push as the only way out of the wait.
		f := node.NewLightRadiusFunc(p, time.Hour, time.Now)
		setAfter(t, p, 5*time.Millisecond, 7, depth.StateKnown)

		got, err := f(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 7 {
			t.Fatalf("got radius %d, want 7", got)
		}

		start := time.Now()
		got, err = f(context.Background())
		if err != nil {
			t.Fatalf("next call: unexpected error: %v", err)
		}
		if got != 7 {
			t.Fatalf("next call: got radius %d, want 7", got)
		}
		if elapsed := time.Since(start); elapsed >= testPrompt {
			t.Fatalf("next call returned after %v, want before %v", elapsed, testPrompt)
		}
	})

	t.Run("no data past the deadline", func(t *testing.T) {
		t.Parallel()

		var offset atomic.Int64
		base := time.Now()
		now := func() time.Time { return base.Add(time.Duration(offset.Load())) }

		p := depthmock.New()
		f := node.NewLightRadiusFunc(p, time.Hour, now)
		offset.Store(int64(time.Hour))

		start := time.Now()
		if _, err := f(context.Background()); !errors.Is(err, depth.ErrUnknown) {
			t.Fatalf("got error %v, want %v", err, depth.ErrUnknown)
		}
		if elapsed := time.Since(start); elapsed >= testPrompt {
			t.Fatalf("returned after %v, want before %v", elapsed, testPrompt)
		}
	})

	t.Run("no data reaches the deadline", func(t *testing.T) {
		t.Parallel()

		p := depthmock.New()
		f := node.NewLightRadiusFunc(p, testRoundLength, time.Now)

		if _, err := f(context.Background()); !errors.Is(err, depth.ErrUnknown) {
			t.Fatalf("got error %v, want %v", err, depth.ErrUnknown)
		}
	})

	t.Run("context canceled during the wait", func(t *testing.T) {
		t.Parallel()

		p := depthmock.New()
		f := node.NewLightRadiusFunc(p, time.Hour, time.Now)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		timer := time.AfterFunc(5*time.Millisecond, cancel)
		defer timer.Stop()

		_, err := f(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got error %v, want %v", err, context.Canceled)
		}
		if errors.Is(err, depth.ErrUnknown) {
			t.Fatalf("got %v, want the context error only", err)
		}
	})
}

func TestFullRadiusFunc(t *testing.T) {
	t.Parallel()

	t.Run("not ready, context canceled", func(t *testing.T) {
		t.Parallel()

		f := node.NewFullRadiusFunc(make(chan struct{}), func() uint8 { return 3 }, 0)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if _, err := f(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("got error %v, want %v", err, context.Canceled)
		}
	})

	for _, tc := range []struct {
		name           string
		local, minimum uint8
		want           uint8
	}{
		{name: "local radius", local: 3, minimum: 0, want: 3},
		{name: "minimum radius", local: 0, minimum: 2, want: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ready := make(chan struct{})
			close(ready)
			f := node.NewFullRadiusFunc(ready, func() uint8 { return tc.local }, tc.minimum)

			got, err := f(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got radius %d, want %d", got, tc.want)
			}
		})
	}
}
