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
	Workers:            8,
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
		done := make(chan struct{})

		overlay := swarm.RandAddress(t)
		oldAddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb")
		newAddr, _ := ma.NewMultiaddr("/ip4/192.168.1.1/tcp/7072/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb")

		pingFunc := func(_ context.Context, a ma.Multiaddr) (time.Duration, error) {
			// Verify that the new address is being pinged, not the old one
			if a.Equal(oldAddr) {
				t.Fatalf("ping should use updated address, got old address")
			}
			if a.Equal(newAddr) {
				done <- struct{}{}
			}
			return 0, nil
		}

		reachableFunc := func(addr swarm.Address, status p2p.ReachabilityStatus) {}

		mock := newMock(pingFunc, reachableFunc)

		r := reacher.New(mock, mock, &defaultOptions, log.Noop)
		testutil.CleanupCloser(t, r)

		// First connection with old address
		r.Connected(overlay, oldAddr)
		// Immediate reconnection with new address (simulates peer reconnecting with different IP)
		r.Connected(overlay, newAddr)

		select {
		case <-time.After(time.Second * 5):
			t.Fatalf("test timed out")
		case <-done:
		}
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
