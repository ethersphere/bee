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

	time.Sleep(time.Millisecond * 50) // wait for reachable func to be called

	select {
	case <-time.After(time.Second):
		t.Fatalf("test time out")
	case <-done:
	}
}

func TestPingFailure(t *testing.T) {

	var (
		want = p2p.ReachabilityStatusPrivate
		done = make(chan struct{})
	)

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

	overlay := test.RandomAddress()

	r.Connected(overlay, nil)

	select {
	case <-time.After(time.Second):
		t.Fatalf("test time out")
	case <-done:
	}
}

func TestDisconnected(t *testing.T) {

	var (
		overlay = test.RandomAddress()
	)

	pingFunc := func(context.Context, ma.Multiaddr) (time.Duration, error) {
		time.Sleep(time.Millisecond * 10) // sleep between calls
		return 0, nil
	}

	reachableFunc := func(addr swarm.Address, b p2p.ReachabilityStatus) {
		if addr.Equal(overlay) {
			t.Fatalf("overlay should be disconnected")
		}
	}

	mock := newMock(pingFunc, reachableFunc)

	r := reacher.New(mock, mock)
	defer r.Close()

	r.Connected(test.RandomAddress(), nil)
	r.Connected(overlay, nil)
	r.Disconnected(overlay)

	time.Sleep(time.Millisecond * 100) // wait for reachable func to be called
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
