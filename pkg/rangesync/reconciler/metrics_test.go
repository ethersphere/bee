// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// The metrics of T6.5 part 2: peer_failures_total by reason, and the stripe
// jobs' queue wait and the first-chunk latency of stripe jobs and catch-up
// fetches.

// TestMetricsRegistered: the reconciler's Metrics report the failures with
// every reason at 0 from the start, as the unlabeled counter was, and the
// queue wait and first-chunk histograms of every kind.
func TestMetricsRegistered(t *testing.T) {
	t.Parallel()

	r := reconciler.New(self, nil, newFakeService(status(1)), log.Noop, reconciler.Options{})
	t.Cleanup(func() { _ = r.Close() })
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(r.Metrics()...)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"bee_rangesync_peer_failures_total":       {"fetch_shortfall", "invalid_message", "other", "stripe_verification", "verifier_withheld"},
		"bee_rangesync_stripe_queue_wait_seconds": {"claim", "own"},
		"bee_rangesync_first_chunk_seconds":       {"catch_up", "claim", "own"},
	}
	label := map[string]string{
		"bee_rangesync_peer_failures_total":       "reason",
		"bee_rangesync_stripe_queue_wait_seconds": "kind",
		"bee_rangesync_first_chunk_seconds":       "kind",
	}
	for _, mf := range mfs {
		labels, ok := want[mf.GetName()]
		if !ok {
			continue
		}
		got := make([]string, 0, len(mf.GetMetric()))
		for _, m := range mf.GetMetric() {
			if len(m.GetLabel()) != 1 || m.GetLabel()[0].GetName() != label[mf.GetName()] {
				t.Fatalf("%s: labels %v, want one, %s", mf.GetName(), m.GetLabel(), label[mf.GetName()])
			}
			got = append(got, m.GetLabel()[0].GetValue())
			if c := m.GetCounter(); c != nil && c.GetValue() != 0 {
				t.Fatalf("%s %v: %v, want 0", mf.GetName(), got, c.GetValue())
			}
			if h := m.GetHistogram(); h != nil && h.GetSampleCount() != 0 {
				t.Fatalf("%s %v: %d observations, want 0", mf.GetName(), got, h.GetSampleCount())
			}
		}
		slices.Sort(got)
		if !slices.Equal(got, labels) {
			t.Fatalf("%s: label values %v, want %v", mf.GetName(), got, labels)
		}
		delete(want, mf.GetName())
	}
	if len(want) > 0 {
		t.Fatalf("not reported: %v", want)
	}
}

// TestPeerFailuresReason: each failure is counted under the reason of the
// sentinel error it wraps, as each caller of Fail reports it: the stripe
// verification's (stripes_verify.go), a fetch shortfall scored or a
// violation (rangesync's shortfall.go), the other violations (rangesync's
// violation and initiatorViolation), and anything else under other.
func TestPeerFailuresReason(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		err    error
		reason string
	}{
		{"stripe verification", fmt.Errorf("stripe %s: 9 missing: %w", fingerprint.NewRange(swarm.ZeroAddress, 0), reconciler.ErrStripeVerification), "stripe_verification"},
		{"verifier withheld", fmt.Errorf("stripe: verifier stored 0 of 5: %w", reconciler.ErrVerifierWithheld), "verifier_withheld"},
		{"fetch shortfall", fmt.Errorf("%w: 9 of 100 requested ids missing again", rangesync.ErrFetchShortfall), "fetch_shortfall"},
		{"blatant fetch shortfall", fmt.Errorf("%w: %w: 60 of 100 requested ids not delivered", rangesync.ErrInvalidMessage, rangesync.ErrFetchShortfall), "fetch_shortfall"},
		{"repeated fetch shortfall", fmt.Errorf("%w: %w, again within 1h", rangesync.ErrInvalidMessage, fmt.Errorf("%w: 9 of 100", rangesync.ErrFetchShortfall)), "fetch_shortfall"},
		{"stalled fetch", fmt.Errorf("%w: no deliveries within 30s", rangesync.ErrInvalidMessage), "invalid_message"},
		{"invalid opening", fmt.Errorf("opening: %w", fmt.Errorf("%w: bad range", rangesync.ErrInvalidMessage)), "invalid_message"},
		{"other", errors.New("something else"), "other"},
		{"nil", nil, "other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := reconciler.New(self, nil, newFakeService(status(1)), log.Noop, reconciler.Options{})
			t.Cleanup(func() { _ = r.Close() })
			r.Fail(higher, tc.err)
			r.Fail(higher, tc.err)
			for _, reason := range []string{"stripe_verification", "verifier_withheld", "fetch_shortfall", "invalid_message", "other"} {
				want := 0.0
				if reason == tc.reason {
					want = 2
				}
				if got := r.PeerFailuresTotal(reason); got != want {
					t.Fatalf("peer_failures_total{reason=%q} %v, want %v", reason, got, want)
				}
			}
		})
	}
}

// TestStripeJobMetrics: the stripe worker observes a job's queue wait, from
// its registration to the worker taking it, once when it starts, by kind
// (own, claim), and its first-chunk latency, from its start to the first
// chunk it stores, once per job; a job that stores nothing observes no
// first chunk.
func TestStripeJobMetrics(t *testing.T) {
	t.Parallel()

	const largeDiff = 100
	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	within := func(t *testing.T, what string, got, from float64) {
		t.Helper()
		if got < from || got >= from+1 {
			t.Fatalf("%s %vs, want %vs and less than a second more", what, got, from)
		}
	}

	t.Run("claim queue wait", func(t *testing.T) {
		t.Parallel()
		staller, honest := higher, addr(0xf1, 0)
		svc := newPinService(status(1000))
		svc.remote[staller.ByteString()] = status(1000)
		svc.remote[honest.ByteString()] = status(1000)
		svc.stall[staller.ByteString()] = true
		h := newPinHarness(t, svc, svc.fakeService, largeDiff, staller, honest)
		waitFor(t, "guard released", h.r.GuardReleased)

		stalled, queued := root.Child(2, 0), root.Child(2, 3)
		if !h.r.EnqueuePeerStripes(staller, []fingerprint.Range{stalled}) {
			t.Fatal("stalled claim not registered")
		}
		waitFor(t, "the stalled claim in flight", func() bool { return stripeSessions(svc.fakeService, staller, stalled) > 0 })
		if n, sum := h.r.StripeQueueWait("claim"); n != 1 || sum >= 1 {
			t.Fatalf("%d queue waits of %vs, want one of less than a second", n, sum)
		}
		if !h.r.EnqueuePeerStripes(honest, []fingerprint.Range{queued}) {
			t.Fatal("queued claim not registered")
		}
		settle()
		if n := stripeSessions(svc.fakeService, honest, queued); n != 0 {
			t.Fatalf("%d stripe sessions of the queued claim", n)
		}
		h.clock.Add(time.Minute)
		h.clock.Add(reconciler.ClaimIdleTimeout) // the stalled claim ends
		waitFor(t, "the queued claim runs", func() bool { return stripeSessions(svc.fakeService, honest, queued) > 0 })
		n, sum := h.r.StripeQueueWait("claim")
		if n != 2 {
			t.Fatalf("%d queue waits, want 2", n)
		}
		within(t, "queue wait", sum, (time.Minute + reconciler.ClaimIdleTimeout).Seconds())
		if n, _ := h.r.StripeQueueWait("own"); n != 0 {
			t.Fatalf("%d own queue waits, want 0", n)
		}
		if n, _ := h.r.FirstChunk("claim"); n != 0 {
			t.Fatalf("%d first chunks of claims that stored nothing", n)
		}
	})

	t.Run("claim first chunk", func(t *testing.T) {
		t.Parallel()
		peer := higher
		svc := newPinService(status(1000))
		svc.remote[peer.ByteString()] = status(1000)
		svc.lists[peer.ByteString()] = 3
		svc.gated[peer.ByteString()] = true
		h := newPinHarness(t, svc, svc.fakeService, largeDiff, peer)
		waitFor(t, "guard released", h.r.GuardReleased)

		rg := root.Child(2, 1)
		if !h.r.EnqueuePeerStripes(peer, []fingerprint.Range{rg}) {
			t.Fatal("claim not registered")
		}
		waitFor(t, "the claim in flight", func() bool { return stripeSessions(svc.fakeService, peer, rg) > 0 })
		h.clock.Add(30 * time.Second)
		svc.drip(t)
		n, sum := h.r.FirstChunk("claim")
		if n != 1 {
			t.Fatalf("%d first chunks, want 1", n)
		}
		within(t, "first chunk", sum, 30)
		svc.drip(t)
		svc.drip(t)
		if n, _ := h.r.FirstChunk("claim"); n != 1 {
			t.Fatalf("%d first chunks after three stored, want 1", n)
		}
		if n, _ := h.r.FirstChunk("own"); n != 0 {
			t.Fatalf("%d own first chunks, want 0", n)
		}
	})

	t.Run("own", func(t *testing.T) {
		t.Parallel()
		a, b := higher, addr(0xf1, 0)
		svc := newPinService(status(0))
		for _, p := range []swarm.Address{a, b} {
			svc.remote[p.ByteString()] = status(1000)
			svc.lists[p.ByteString()] = 3
			svc.gated[p.ByteString()] = true
		}
		h := newPinHarness(t, svc, svc.fakeService, 0, a, b)
		waitFor(t, "bootstrap in flight", func() bool {
			_, own, ok := h.r.RunningStripe()
			return ok && own
		})
		if n, sum := h.r.StripeQueueWait("own"); n != 1 || sum >= 1 {
			t.Fatalf("%d own queue waits of %vs, want one of less than a second", n, sum)
		}
		h.clock.Add(45 * time.Second)
		svc.drip(t)
		n, sum := h.r.FirstChunk("own")
		if n != 1 {
			t.Fatalf("%d first chunks, want 1", n)
		}
		within(t, "first chunk", sum, 45)
		svc.drip(t)
		if n, _ := h.r.FirstChunk("own"); n != 1 {
			t.Fatalf("%d first chunks after two stored, want 1", n)
		}
		if n, _ := h.r.StripeQueueWait("claim"); n != 0 {
			t.Fatalf("%d claim queue waits, want 0", n)
		}
	})
}

// TestCatchUpFirstChunkMetric: a catch-up fetch (CatchUpMinDiff wants or
// more), of a session this node served (Wants) or opened (a round), observes
// its first-chunk latency, from its start to its first stored chunk, once;
// a smaller fetch observes nothing.
func TestCatchUpFirstChunkMetric(t *testing.T) {
	t.Parallel()

	t.Run("served", func(t *testing.T) {
		t.Parallel()
		small, large := higher, addr(0xf1, 0)
		svc := newPinService(status(1000))
		svc.remote[small.ByteString()] = status(1000)
		svc.remote[large.ByteString()] = status(1000)
		svc.gated[large.ByteString()] = true
		h := newPinHarness(t, svc, svc.fakeService, 0, small, large)

		h.r.Wants(small, make([]storer.ChunkID, reconciler.CatchUpMinDiff-1))
		waitFor(t, "the small fetch", func() bool { return svc.count(svc.fetched, small) > 0 })
		settle()
		if n, _ := h.r.FirstChunk("catch_up"); n != 0 {
			t.Fatalf("%d first chunks of a fetch below CatchUpMinDiff", n)
		}

		h.r.Wants(large, make([]storer.ChunkID, reconciler.CatchUpMinDiff))
		call := nextGatedFetch(t, svc)
		h.clock.Add(1500 * time.Millisecond)
		close(call.release)
		if stored := <-call.done; stored == 0 {
			t.Fatal("the catch-up stored nothing")
		}
		n, sum := h.r.FirstChunk("catch_up")
		if n != 1 {
			t.Fatalf("%d first chunks, want 1", n)
		}
		if sum < 1.5 || sum >= 2 {
			t.Fatalf("first chunk %vs, want 1.5s", sum)
		}
	})

	t.Run("round", func(t *testing.T) {
		t.Parallel()
		svc := newPinService(status(1000))
		svc.remote[higher.ByteString()] = status(1001)
		svc.wants[higher.ByteString()] = reconciler.CatchUpMinDiff
		svc.gated[higher.ByteString()] = true
		h := newPinHarness(t, svc, svc.fakeService, 0, higher)

		call := nextGatedFetch(t, svc)
		svc.set(func() { // no further sessions find wants
			svc.remote[higher.ByteString()] = status(1000)
			svc.wants[higher.ByteString()] = 0
		})
		h.clock.Add(1500 * time.Millisecond)
		close(call.release)
		if stored := <-call.done; stored == 0 {
			t.Fatal("the catch-up stored nothing")
		}
		n, sum := h.r.FirstChunk("catch_up")
		if n != 1 {
			t.Fatalf("%d first chunks, want 1", n)
		}
		if sum < 1.5 || sum >= 2 {
			t.Fatalf("first chunk %vs, want 1.5s", sum)
		}
		settle()
		if n, _ := h.r.FirstChunk("catch_up"); n != 1 {
			t.Fatalf("%d first chunks after the catch-up, want 1", n)
		}
	})
}

// nextGatedFetch returns the next fetch of a gated peer of svc, waiting for
// the test.
func nextGatedFetch(t *testing.T, svc *pinService) gatedFetch {
	t.Helper()
	select {
	case call := <-svc.calls:
		return call
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a gated fetch")
		return gatedFetch{}
	}
}

// TestCatchUpFirstChunkStripeRetry (T6.5 part 2 fix round 1): the retry of
// IDs a stripe's peer did not deliver, from another peer known to hold them,
// is a catch-up fetch but belongs to the stripe's job: it observes no
// catch_up first chunk. The retry of a session's catch-up fetch does
// (TestWantSetCatchUpRetry).
func TestCatchUpFirstChunkStripeRetry(t *testing.T) {
	t.Parallel()

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	x := testChunkID(1)
	svc := newWantService(status(1))
	svc.lists(higher, x)
	svc.withholds(higher, x) // it lists x and does not deliver it
	svc.lists(higher2, x)
	// higher2 holds x but lists nothing in its sessions, so that the stripe,
	// reassigned to it, does not fetch x: the retry does
	svc.set(func() { svc.wantIDs[higher2.ByteString()] = nil })
	claimed := svc.claimSignal(higher)
	pause := make(chan struct{})
	svc.pause[higher.ByteString()] = pause
	h := newWantHarness(t, svc, higher, higher2)
	h.r.SetStripeRetryInterval(time.Millisecond)
	svc.set(func() { // equal digests: no round session
		svc.remote[higher.ByteString()] = status(1)
		svc.remote[higher2.ByteString()] = status(1)
	})
	h.r.Start(context.Background())
	waitState(t, h.r, higher, reconciler.StateIdle)
	waitState(t, h.r, higher2, reconciler.StateIdle)

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}})
	}()
	waitClosed(t, "the stripe's fetch claimed x", claimed)
	// a served session's fetch from higher2, below CatchUpMinDiff, is
	// denied x, which records higher2 as its source
	h.r.Wants(higher2, []storer.ChunkID{x})
	waitFor(t, "higher2 recorded as a source", func() bool { return h.r.SourceIDs() == 1 })
	close(pause) // the stripe's fetch ends with x undelivered: retried from higher2
	waitDelivered(t, svc, x)
	assertDelivered(t, svc, x, higher2)
	waitClosed(t, "the stripe transfer", done)
	settle()
	if n := len(svc.fetchesFrom(higher2)); n != 2 {
		t.Fatalf("%d fetches from higher2, want 2: the served session's and the retry", n)
	}
	if n, _ := h.r.FirstChunk("catch_up"); n != 0 {
		t.Fatalf("%d catch_up first chunks of a stripe's retry, want 0", n)
	}
}
