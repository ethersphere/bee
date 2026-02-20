// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reacher_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"testing/synctest"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/reacher"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	ma "github.com/multiformats/go-multiaddr"
	"go.uber.org/atomic"
)

var defaultOptions = reacher.Options{
	PingTimeout:        time.Second * 5,
	Workers:            4,
	RetryAfterDuration: time.Second,
}

func TestPingSuccess(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		pingFunc      func(context.Context, ma.Multiaddr) (time.Duration, error)
		reachableFunc func(chan struct{}) func(addr swarm.Address, got p2p.ReachabilityStatus)
	}{
		{
			name: "ping success",
			pingFunc: func(context.Context, ma.Multiaddr) (time.Duration, error) {
				return 0, nil
			},
			reachableFunc: func(done chan struct{}) func(addr swarm.Address, got p2p.ReachabilityStatus) {
				return func(addr swarm.Address, got p2p.ReachabilityStatus) {
					if got != p2p.ReachabilityStatusPublic {
						t.Fatalf("got %v, want %v", got, p2p.ReachabilityStatusPublic)
					}
					done <- struct{}{}
				}
			},
		},
		{
			name: "ping failure",
			pingFunc: func(context.Context, ma.Multiaddr) (time.Duration, error) {
				return 0, errors.New("test error")
			},
			reachableFunc: func(done chan struct{}) func(addr swarm.Address, got p2p.ReachabilityStatus) {
				return func(addr swarm.Address, got p2p.ReachabilityStatus) {
					if got != p2p.ReachabilityStatusPrivate {
						t.Fatalf("got %v, want %v", got, p2p.ReachabilityStatusPrivate)
					}
					done <- struct{}{}
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				done := make(chan struct{})
				mock := newMock(tc.pingFunc, tc.reachableFunc(done))

				r := reacher.New(mock, mock, &defaultOptions, log.Noop)
				testutil.CleanupCloser(t, r)

				overlay := swarm.RandAddress(t)
				addr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb")

				r.Connected(overlay, addr)

				select {
				case <-time.After(time.Second * 5):
					t.Fatalf("test timed out")
				case <-done:
				}
			})
		})
	}
}

func TestDisconnected(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var (
			disconnectedOverlay = swarm.RandAddress(t)
			disconnectedMa, _   = ma.NewMultiaddr("/ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb")
		)

		/*
			Because the Disconnected is called after Connected, it may be that one of the workers
			have picked up the peer already. So to test that the Disconnected really works,
			if the ping function pings the peer we are trying to disconnect, we return an error
			which triggers another attempt in the future, which by the, the peer should already be removed.
		*/

		var errs atomic.Int64
		pingFunc := func(_ context.Context, a ma.Multiaddr) (time.Duration, error) {
			if a != nil && a.Equal(disconnectedMa) {
				errs.Inc()
				if errs.Load() > 1 {
					t.Fatalf("overlay should be disconnected already")
				}
				return 0, errors.New("test error")
			}
			return 0, nil
		}

		reachableFunc := func(addr swarm.Address, b p2p.ReachabilityStatus) {}

		mock := newMock(pingFunc, reachableFunc)

		r := reacher.New(mock, mock, &defaultOptions, log.Noop)
		testutil.CleanupCloser(t, r)

		addr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/7072/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb")
		r.Connected(swarm.RandAddress(t), addr)
		r.Connected(disconnectedOverlay, disconnectedMa)
		r.Disconnected(disconnectedOverlay)
	})
}

func TestAddressUpdateOnReconnect(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		// Use 1 worker and no jitter to make timing deterministic.
		options := reacher.Options{
			PingTimeout:        time.Second * 5,
			Workers:            1,
			RetryAfterDuration: time.Minute,
			JitterFactor:       0,
		}

		overlay := swarm.RandAddress(t)
		oldAddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb")
		newAddr, _ := ma.NewMultiaddr("/ip4/192.168.1.1/tcp/7072/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb")

		var pingsMu sync.Mutex
		var pings []ma.Multiaddr
		pinged := make(chan struct{}, 8)

		pingFunc := func(_ context.Context, a ma.Multiaddr) (time.Duration, error) {
			pingsMu.Lock()
			pings = append(pings, a)
			pingsMu.Unlock()
			pinged <- struct{}{}
			return 0, nil
		}

		reachableFunc := func(addr swarm.Address, status p2p.ReachabilityStatus) {}

		mock := newMock(pingFunc, reachableFunc)

		r := reacher.New(mock, mock, &options, log.Noop)
		testutil.CleanupCloser(t, r)

		// First connection with old address – triggers initial ping.
		r.Connected(overlay, oldAddr)

		select {
		case <-time.After(time.Second * 10):
			t.Fatal("timed out waiting for initial ping")
		case <-pinged:
		}

		// Verify old address was pinged first.
		pingsMu.Lock()
		if len(pings) != 1 {
			t.Fatalf("expected 1 ping after initial connect, got %d", len(pings))
		}
		if !pings[0].Equal(oldAddr) {
			t.Fatalf("first ping should use old address, got %s", pings[0])
		}
		pingsMu.Unlock()

		// Reconnect with a new address — should trigger immediate re-ping.
		r.Connected(overlay, newAddr)

		select {
		case <-time.After(time.Second * 10):
			t.Fatal("timed out waiting for reconnect ping")
		case <-pinged:
		}

		// Verify the reconnect pinged the new address.
		pingsMu.Lock()
		if len(pings) != 2 {
			t.Fatalf("expected 2 pings after reconnect, got %d", len(pings))
		}
		if !pings[1].Equal(newAddr) {
			t.Fatalf("reconnect ping should use new address, got %s", pings[1])
		}
		pingsMu.Unlock()

		// After reconnect success, successCount=1 so backoff = 2min (no jitter).
		// Sleep past 2min to trigger the scheduled re-ping.
		time.Sleep(3 * time.Minute)

		select {
		case <-time.After(time.Second * 10):
			t.Fatal("timed out waiting for scheduled re-ping")
		case <-pinged:
		}

		// Verify the scheduled re-ping used the new address.
		pingsMu.Lock()
		if len(pings) != 3 {
			t.Fatalf("expected 3 pings after retry duration, got %d", len(pings))
		}
		if !pings[2].Equal(newAddr) {
			t.Fatalf("scheduled re-ping should use new address, got %s", pings[2])
		}
		pingsMu.Unlock()
	})
}

func TestHeapOrdering(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		// Use single worker to ensure sequential processing
		options := reacher.Options{
			PingTimeout:        time.Second * 5,
			Workers:            1,
			RetryAfterDuration: time.Second * 10,
		}

		var pingOrder []swarm.Address
		var pingOrderMu sync.Mutex
		allPinged := make(chan struct{})

		overlay1 := swarm.RandAddress(t)
		overlay2 := swarm.RandAddress(t)
		overlay3 := swarm.RandAddress(t)
		addr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb")

		pingFunc := func(_ context.Context, _ ma.Multiaddr) (time.Duration, error) {
			return 0, nil
		}

		reachableFunc := func(overlay swarm.Address, status p2p.ReachabilityStatus) {
			pingOrderMu.Lock()
			pingOrder = append(pingOrder, overlay)
			if len(pingOrder) == 3 {
				close(allPinged)
			}
			pingOrderMu.Unlock()
		}

		mock := newMock(pingFunc, reachableFunc)

		r := reacher.New(mock, mock, &options, log.Noop)
		testutil.CleanupCloser(t, r)

		// Add peers - they should all be pinged since retryAfter starts at zero
		r.Connected(overlay1, addr)
		r.Connected(overlay2, addr)
		r.Connected(overlay3, addr)

		select {
		case <-time.After(time.Second * 5):
			t.Fatalf("test timed out, only %d peers pinged", len(pingOrder))
		case <-allPinged:
		}

		// Verify all three peers were pinged
		pingOrderMu.Lock()
		defer pingOrderMu.Unlock()

		if len(pingOrder) != 3 {
			t.Fatalf("expected 3 peers pinged, got %d", len(pingOrder))
		}

		// Verify all overlays are present (order may vary due to heap with same retryAfter)
		seen := make(map[string]bool)
		for _, o := range pingOrder {
			seen[o.String()] = true
		}
		if !seen[overlay1.String()] || !seen[overlay2.String()] || !seen[overlay3.String()] {
			t.Fatalf("not all peers were pinged")
		}
	})
}

func TestBackoffOnFailure(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		options := reacher.Options{
			PingTimeout:        time.Second * 5,
			Workers:            1,
			RetryAfterDuration: time.Minute, // base interval
			JitterFactor:       0,
		}

		overlay := swarm.RandAddress(t)
		addr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb")

		var pingsMu sync.Mutex
		var pingTimes []time.Time
		pinged := make(chan struct{}, 16)

		pingFunc := func(_ context.Context, _ ma.Multiaddr) (time.Duration, error) {
			pingsMu.Lock()
			pingTimes = append(pingTimes, time.Now())
			pingsMu.Unlock()
			pinged <- struct{}{}
			return 0, errors.New("always fail")
		}

		reachableFunc := func(addr swarm.Address, status p2p.ReachabilityStatus) {}

		mock := newMock(pingFunc, reachableFunc)

		r := reacher.New(mock, mock, &options, log.Noop)
		testutil.CleanupCloser(t, r)

		r.Connected(overlay, addr)

		// Wait for the first ping (immediate).
		select {
		case <-time.After(time.Second * 10):
			t.Fatal("timed out waiting for first ping")
		case <-pinged:
		}

		// After first failure: backoff = 2min (no jitter).
		// Sleep 90s which is less than the 2min backoff.
		time.Sleep(90 * time.Second)

		pingsMu.Lock()
		count := len(pingTimes)
		pingsMu.Unlock()
		if count != 1 {
			t.Fatalf("expected 1 ping after 90s (backoff=2min), got %d", count)
		}

		// Sleep past the 2min backoff to trigger the second ping.
		time.Sleep(55 * time.Second)

		select {
		case <-time.After(time.Second * 10):
			t.Fatal("timed out waiting for second ping")
		case <-pinged:
		}

		pingsMu.Lock()
		count = len(pingTimes)
		pingsMu.Unlock()
		if count != 2 {
			t.Fatalf("expected 2 pings after 2min backoff, got %d", count)
		}

		// After second failure: backoff = 4min (no jitter).
		// Sleep past 4min to trigger the third ping.
		time.Sleep(5 * time.Minute)

		select {
		case <-time.After(time.Second * 10):
			t.Fatal("timed out waiting for third ping")
		case <-pinged:
		}

		pingsMu.Lock()
		count = len(pingTimes)
		pingsMu.Unlock()
		if count != 3 {
			t.Fatalf("expected 3 pings after 4min backoff, got %d", count)
		}
	})
}

func TestBackoffOnSuccess(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		options := reacher.Options{
			PingTimeout:        time.Second * 5,
			Workers:            1,
			RetryAfterDuration: time.Minute,
			JitterFactor:       0,
		}

		overlay := swarm.RandAddress(t)
		addr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb")

		var pingsMu sync.Mutex
		var pingCount int
		pinged := make(chan struct{}, 16)

		pingFunc := func(_ context.Context, _ ma.Multiaddr) (time.Duration, error) {
			pingsMu.Lock()
			pingCount++
			pingsMu.Unlock()
			pinged <- struct{}{}
			return 0, nil // always succeed
		}

		reachableFunc := func(addr swarm.Address, status p2p.ReachabilityStatus) {}

		mock := newMock(pingFunc, reachableFunc)

		r := reacher.New(mock, mock, &options, log.Noop)
		testutil.CleanupCloser(t, r)

		r.Connected(overlay, addr)

		// First ping (immediate).
		select {
		case <-time.After(time.Second * 10):
			t.Fatal("timed out waiting for first ping")
		case <-pinged:
		}

		// After first success: backoff = 2min (no jitter).
		// Sleep 90s which is less than 2min backoff.
		time.Sleep(90 * time.Second)

		pingsMu.Lock()
		if pingCount != 1 {
			t.Fatalf("expected 1 ping after 90s (backoff=2min), got %d", pingCount)
		}
		pingsMu.Unlock()

		// Sleep past the 2min backoff to trigger the second ping.
		time.Sleep(55 * time.Second)

		select {
		case <-time.After(time.Second * 10):
			t.Fatal("timed out waiting for second ping")
		case <-pinged:
		}

		pingsMu.Lock()
		if pingCount != 2 {
			t.Fatalf("expected 2 pings after 2min backoff, got %d", pingCount)
		}
		pingsMu.Unlock()

		// After second success: successCount=2 (capped at 2), backoff = 4min (no jitter).
		// Sleep past 4min to trigger the third ping.
		time.Sleep(5 * time.Minute)

		select {
		case <-time.After(time.Second * 10):
			t.Fatal("timed out waiting for third ping")
		case <-pinged:
		}

		pingsMu.Lock()
		if pingCount != 3 {
			t.Fatalf("expected 3 pings after capped 4min backoff, got %d", pingCount)
		}
		pingsMu.Unlock()

		// Third success: still capped at 4min (no jitter).
		// Sleep 1min which is well below 4min.
		time.Sleep(time.Minute)

		pingsMu.Lock()
		if pingCount != 3 {
			t.Fatalf("expected still 3 pings after 1min (capped backoff=4min), got %d", pingCount)
		}
		pingsMu.Unlock()

		// Sleep past the 4min backoff to confirm cap holds.
		time.Sleep(4 * time.Minute)

		select {
		case <-time.After(time.Second * 10):
			t.Fatal("timed out waiting for fourth ping")
		case <-pinged:
		}

		pingsMu.Lock()
		if pingCount != 4 {
			t.Fatalf("expected 4 pings after capped backoff, got %d", pingCount)
		}
		pingsMu.Unlock()
	})
}

func TestBackoffResetOnReconnect(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		options := reacher.Options{
			PingTimeout:        time.Second * 5,
			Workers:            1,
			RetryAfterDuration: time.Minute,
			JitterFactor:       0,
		}

		overlay := swarm.RandAddress(t)
		addr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb")

		var pingsMu sync.Mutex
		var pingCount int
		pinged := make(chan struct{}, 16)

		pingFunc := func(_ context.Context, _ ma.Multiaddr) (time.Duration, error) {
			pingsMu.Lock()
			pingCount++
			pingsMu.Unlock()
			pinged <- struct{}{}
			return 0, errors.New("always fail")
		}

		reachableFunc := func(addr swarm.Address, status p2p.ReachabilityStatus) {}

		mock := newMock(pingFunc, reachableFunc)

		r := reacher.New(mock, mock, &options, log.Noop)
		testutil.CleanupCloser(t, r)

		// First connection — immediate ping, then fail.
		r.Connected(overlay, addr)

		select {
		case <-time.After(time.Second * 10):
			t.Fatal("timed out waiting for first ping")
		case <-pinged:
		}

		// After first failure, backoff = 2min.
		// Reconnect resets failCount and triggers immediate re-ping.
		r.Connected(overlay, addr)

		select {
		case <-time.After(time.Second * 10):
			t.Fatal("timed out waiting for reconnect ping")
		case <-pinged:
		}

		// After reconnect failure, backoff should be 2min again (not 4min),
		// because failCount was reset. Sleep past 2min.
		time.Sleep(3 * time.Minute)

		select {
		case <-time.After(time.Second * 10):
			t.Fatal("timed out waiting for post-reconnect retry")
		case <-pinged:
		}

		pingsMu.Lock()
		if pingCount != 3 {
			t.Fatalf("expected 3 pings (initial + reconnect + retry), got %d", pingCount)
		}
		pingsMu.Unlock()
	})
}

type mock struct {
	pingFunc      func(context.Context, ma.Multiaddr) (time.Duration, error)
	reachableFunc func(swarm.Address, p2p.ReachabilityStatus)
}

func newMock(ping func(context.Context, ma.Multiaddr) (time.Duration, error), reach func(swarm.Address, p2p.ReachabilityStatus)) *mock {
	return &mock{
		pingFunc:      ping,
		reachableFunc: reach,
	}
}

func (m *mock) Ping(ctx context.Context, addr ma.Multiaddr) (time.Duration, error) {
	return m.pingFunc(ctx, addr)
}

func (m *mock) Reachable(addr swarm.Address, status p2p.ReachabilityStatus) {
	m.reachableFunc(addr, status)
}
