// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reacher_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/reacher"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
	ma "github.com/multiformats/go-multiaddr"
	"go.uber.org/atomic"
)

func TestPingSuccess(t *testing.T) {

	var (
		want = p2p.ReachabilityStatusPublic
		done = make(chan struct{})
	)

	pingFunc := func(context.Context, ma.Multiaddr) (time.Duration, error) {
		return 0, nil
	}

	reachableFunc := func(addr swarm.Address, got p2p.ReachabilityStatus) {
		if got != want {
			t.Fatalf("got %v, want %v", got, want)
		}
		close(done)
	}

	mock := newMock(pingFunc, reachableFunc)

	r := reacher.New(mock, mock)
	defer r.Close()

	overlay := test.RandomAddress()

	r.Connected(overlay, nil)

	select {
	case <-time.After(time.Second * 5):
		t.Fatalf("test timed out")
	case <-done:
	}
}

func TestPingFailure(t *testing.T) {

	var (
		want = p2p.ReachabilityStatusPrivate
		done = make(chan struct{})
	)

	defer func(t time.Duration) {
		*reacher.RetryAfter = t
	}(*reacher.RetryAfter)
	*reacher.RetryAfter = time.Millisecond

	pingFunc := func(context.Context, ma.Multiaddr) (time.Duration, error) {
		return 0, errors.New("test error")
	}

	reachableFunc := func(addr swarm.Address, b p2p.ReachabilityStatus) {
		if b != want {
			t.Fatalf("got %v, want %v", b, want)
		}
		close(done)
	}

	mock := newMock(pingFunc, reachableFunc)

	r := reacher.New(mock, mock)
	defer r.Close()

	r.Connected(test.RandomAddress(), nil)

	select {
	case <-time.After(time.Second * 5):
		t.Fatalf("test timed out")
	case <-done:
	}
}

func TestDisconnected(t *testing.T) {

	var (
		disconnectedOverlay = test.RandomAddress()
		disconnectedMa, _   = ma.NewMultiaddr("/ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb")
	)

	defer func(t time.Duration) {
		*reacher.RetryAfter = t
	}(*reacher.RetryAfter)
	*reacher.RetryAfter = time.Millisecond

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

	r := reacher.New(mock, mock)
	defer r.Close()

	r.Connected(test.RandomAddress(), nil)
	r.Connected(disconnectedOverlay, disconnectedMa)
	r.Disconnected(disconnectedOverlay)

	time.Sleep(time.Millisecond * 50) // wait for reachable func to be called
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
