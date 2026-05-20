// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node_test

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/node"
)

// trackingCloser records whether Close was called and, optionally, returns a
// fixed error. It is safe for concurrent use because Shutdown closes several
// components in parallel.
type trackingCloser struct {
	closed atomic.Bool
	err    error
}

func (t *trackingCloser) Close() error {
	t.closed.Store(true)
	return t.err
}

func TestShutdown_NilCloserFieldsAreSafe(t *testing.T) {
	t.Parallel()

	b, ctx := node.NewBeeForShutdownTest(log.Noop, node.ShutdownTestClosers{})

	if err := b.Shutdown(); err != nil {
		t.Fatalf("Shutdown on a Bee with no closers returned %v, want nil", err)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("ctxCancel was not invoked by Shutdown")
	}
}

func TestShutdown_CallsEveryNonNilCloser(t *testing.T) {
	t.Parallel()

	// Cover representative closers from each of Shutdown's phases:
	// parallel first phase, sequential phase, parallel chain phase, and
	// the final sequential tail.
	closers := map[string]*trackingCloser{
		"api":                {},
		"pss":                {},
		"gsoc":               {},
		"pusher":             {},
		"puller":             {},
		"accounting":         {},
		"pullSync":           {},
		"hive":               {},
		"salud":              {},
		"p2p":                {},
		"priceOracle":        {},
		"transactionMonitor": {},
		"transaction":        {},
		"listener":           {},
		"postageService":     {},
		"accesscontrol":      {},
		"tracer":             {},
		"topology":           {},
		"storageIncentives":  {},
		"stabilization":      {},
		"localstore":         {},
		"stateStore":         {},
		"stamperStore":       {},
		"resolver":           {},
	}

	var ethClientCalled atomic.Bool
	b, _ := node.NewBeeForShutdownTest(log.Noop, node.ShutdownTestClosers{
		API:                closers["api"],
		PSS:                closers["pss"],
		GSOC:               closers["gsoc"],
		Pusher:             closers["pusher"],
		Puller:             closers["puller"],
		Accounting:         closers["accounting"],
		PullSync:           closers["pullSync"],
		Hive:               closers["hive"],
		Salud:              closers["salud"],
		P2P:                closers["p2p"],
		PriceOracle:        closers["priceOracle"],
		TransactionMonitor: closers["transactionMonitor"],
		Transaction:        closers["transaction"],
		Listener:           closers["listener"],
		PostageService:     closers["postageService"],
		AccessControl:      closers["accesscontrol"],
		Tracer:             closers["tracer"],
		Topology:           closers["topology"],
		StorageIncentives:  closers["storageIncentives"],
		Stabilization:      closers["stabilization"],
		Localstore:         closers["localstore"],
		StateStore:         closers["stateStore"],
		StamperStore:       closers["stamperStore"],
		Resolver:           closers["resolver"],
		EthClient:          func() { ethClientCalled.Store(true) },
	})

	if err := b.Shutdown(); err != nil {
		t.Fatalf("Shutdown returned unexpected error: %v", err)
	}

	for name, c := range closers {
		if !c.closed.Load() {
			t.Errorf("closer %q was not invoked", name)
		}
	}
	if !ethClientCalled.Load() {
		t.Error("ethClientCloser was not invoked")
	}
}

func TestShutdown_AggregatesErrors(t *testing.T) {
	t.Parallel()

	apiErr := errors.New("api boom")
	stateErr := errors.New("statestore boom")
	tracerErr := errors.New("tracer boom")

	b, _ := node.NewBeeForShutdownTest(log.Noop, node.ShutdownTestClosers{
		API:        &trackingCloser{err: apiErr},
		StateStore: &trackingCloser{err: stateErr},
		Tracer:     &trackingCloser{err: tracerErr},
	})

	err := b.Shutdown()
	if err == nil {
		t.Fatal("expected aggregated error, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"api", "statestore", "tracer"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected aggregated error to mention %q, got %q", want, msg)
		}
	}
}

func TestShutdown_SecondCallReturnsErrShutdownInProgress(t *testing.T) {
	t.Parallel()

	b, _ := node.NewBeeForShutdownTest(log.Noop, node.ShutdownTestClosers{})

	if err := b.Shutdown(); err != nil {
		t.Fatalf("first Shutdown returned %v, want nil", err)
	}
	if err := b.Shutdown(); !errors.Is(err, node.ErrShutdownInProgress) {
		t.Fatalf("second Shutdown returned %v, want ErrShutdownInProgress", err)
	}
}

func TestShutdown_ConcurrentCallsExactlyOneRuns(t *testing.T) {
	t.Parallel()

	// All shutdown work is no-op (no closers set), so the only way to tell
	// who got the "first" slot is by the returned error: exactly one caller
	// should see nil and the rest should see ErrShutdownInProgress.
	b, _ := node.NewBeeForShutdownTest(log.Noop, node.ShutdownTestClosers{})

	const callers = 8
	var (
		wg       sync.WaitGroup
		nilCount atomic.Int32
		errCount atomic.Int32
	)
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			if err := b.Shutdown(); err == nil {
				nilCount.Add(1)
			} else if errors.Is(err, node.ErrShutdownInProgress) {
				errCount.Add(1)
			} else {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := nilCount.Load(); got != 1 {
		t.Fatalf("expected exactly 1 caller to run shutdown, got %d", got)
	}
	if got := errCount.Load(); got != callers-1 {
		t.Fatalf("expected %d callers to get ErrShutdownInProgress, got %d", callers-1, got)
	}
}
