package reacher_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/reacher"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
	ma "github.com/multiformats/go-multiaddr"
)

func TestPingSuccess(t *testing.T) {

	var (
		got  = false
		want = true
		mu   = sync.Mutex{}
	)

	pingFunc := func(context.Context, ma.Multiaddr) (time.Duration, error) {
		return 0, nil
	}

	reachableFunc := func(addr swarm.Address, b bool) {
		mu.Lock()
		got = b
		mu.Unlock()
	}

	mock := newMock(pingFunc, reachableFunc)

	r := reacher.New(mock, mock)
	defer r.Close()

	overlay := test.RandomAddress()

	r.Connected(overlay, nil)

	time.Sleep(time.Millisecond * 50) // wait for reachable func to be called

	mu.Lock()
	defer mu.Unlock()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPingFailure(t *testing.T) {

	var (
		got  = true
		want = false
		mu   = sync.Mutex{}
	)

	pingFunc := func(context.Context, ma.Multiaddr) (time.Duration, error) {
		return 0, errors.New("test error")
	}

	reachableFunc := func(addr swarm.Address, b bool) {
		mu.Lock()
		got = b
		mu.Unlock()
	}

	mock := newMock(pingFunc, reachableFunc)

	r := reacher.New(mock, mock)
	defer r.Close()

	overlay := test.RandomAddress()

	r.Connected(overlay, nil)

	time.Sleep(time.Millisecond * 50) // wait for reachable func to be called

	mu.Lock()
	defer mu.Unlock()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDisconnected(t *testing.T) {

	var (
		overlay     = test.RandomAddress()
		seenOverlay = false
		mu          = sync.Mutex{}
	)

	pingFunc := func(context.Context, ma.Multiaddr) (time.Duration, error) {
		time.Sleep(time.Millisecond * 5)
		return 0, nil
	}

	reachableFunc := func(addr swarm.Address, b bool) {
		mu.Lock()
		if addr.Equal(overlay) {
			seenOverlay = true
		}
		mu.Unlock()
	}

	mock := newMock(pingFunc, reachableFunc)

	r := reacher.New(mock, mock)
	defer r.Close()

	r.Connected(test.RandomAddress(), nil)
	r.Connected(overlay, nil)
	r.Disconnected(overlay)

	time.Sleep(time.Millisecond * 50) // wait for reachable func to be called

	mu.Lock()
	defer mu.Unlock()
	if seenOverlay {
		t.Fatalf("got %v, want %v", seenOverlay, false)
	}
}

type mock struct {
	pingFunc      func(context.Context, ma.Multiaddr) (time.Duration, error)
	reachableFunc func(swarm.Address, bool)
}

func newMock(ping func(context.Context, ma.Multiaddr) (time.Duration, error), reach func(swarm.Address, bool)) *mock {
	return &mock{
		pingFunc:      ping,
		reachableFunc: reach,
	}
}

func (m *mock) Ping(ctx context.Context, addr ma.Multiaddr) (time.Duration, error) {
	return m.pingFunc(ctx, addr)
}

func (m *mock) Reachable(addr swarm.Address, b bool) {
	m.reachableFunc(addr, b)
}
