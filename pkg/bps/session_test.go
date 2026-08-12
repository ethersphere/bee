// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/bps"
	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	bpstesting "github.com/ethersphere/bee/v2/pkg/bps/testing"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// newClient returns a client service whose streamer routes to broker.
func newClient(t *testing.T, broker *bps.Service, brokerAddr swarm.Address) *bps.Service {
	t.Helper()

	recorder := streamtest.New(
		streamtest.WithProtocols(broker.Protocol()),
		streamtest.WithBaseAddr(brokerAddr),
	)
	client := bps.New(recorder, log.Noop, bps.Options{})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Fatal(err)
		}
	})
	return client
}

func TestSessionOpenAndSubscribe(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	broker, _, brokerAddr := newBroker(t, bps.Options{})
	client := newClient(t, broker, brokerAddr)

	spec := validSpec()
	pub, err := client.Open(ctx, brokerAddr, spec, &pb.PublisherAuth{Owner: spec.Admin})
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	if !pub.Topic().Equal(swarm.NewAddress(spec.Topic)) {
		t.Fatalf("topic: got %s", pub.Topic())
	}
	if !bps.SpecEqual(pub.Spec(), spec) {
		t.Fatal("session did not retain the echoed spec")
	}

	sub, err := client.Subscribe(ctx, brokerAddr, swarm.NewAddress(spec.Topic), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	if !bps.SpecEqual(sub.Spec(), spec) {
		t.Fatal("subscriber did not learn the spec from the Ack echo")
	}
}

func TestSessionRefusal(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	broker, _, brokerAddr := newBroker(t, bps.Options{})
	client := newClient(t, broker, brokerAddr)

	spec := validSpec()
	_, err := client.Subscribe(ctx, brokerAddr, swarm.NewAddress(spec.Topic), nil)
	if !errors.Is(err, bps.ErrRefused) {
		t.Fatalf("got %v want %v", err, bps.ErrRefused)
	}
	var refusal *bps.RefusalError
	if !errors.As(err, &refusal) || refusal.Status != pb.Status_UNKNOWN_TOPIC {
		t.Fatalf("got %v want UNKNOWN_TOPIC", err)
	}
}

func TestSessionPublishRequiresPublisherRole(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	broker, _, brokerAddr := newBroker(t, bps.Options{})
	client := newClient(t, broker, brokerAddr)

	signer, owner := bpstesting.NewSigner(t)
	s := bpstesting.AnchorSOC(t, signer, topic(0x31), []byte("payload"))
	anchor, err := s.Address()
	if err != nil {
		t.Fatal(err)
	}
	spec := &pb.CohortSpec{
		Topic:      anchor.Bytes(),
		Binding:    pb.TopicBinding_ANCHOR,
		Publishers: pb.PublisherRegime_EXPLICIT_SINGLE,
		Admin:      owner,
	}

	pub, err := client.Open(ctx, brokerAddr, spec, &pb.PublisherAuth{Owner: owner})
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	sub, err := client.Subscribe(ctx, brokerAddr, anchor, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	if err := sub.Publish(ctx, s); !errors.Is(err, bps.ErrNotPublisher) {
		t.Fatalf("got %v want %v", err, bps.ErrNotPublisher)
	}
}

// TestServiceCloseTearsDownLiveSessions ensures that closing a Service with
// an open, unclosed session does not leak: Close must tear down live
// sessions itself, and each session's Close waits for its own read-loop
// goroutine to actually return, all well within Close's 5-second budget.
// This test constructs its own client service, rather than using newClient,
// because it deliberately calls Close itself instead of relying on
// t.Cleanup.
func TestServiceCloseTearsDownLiveSessions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	broker, _, brokerAddr := newBroker(t, bps.Options{})

	recorder := streamtest.New(
		streamtest.WithProtocols(broker.Protocol()),
		streamtest.WithBaseAddr(brokerAddr),
	)
	client := bps.New(recorder, log.Noop, bps.Options{})

	spec := validSpec()
	_, err := client.Open(ctx, brokerAddr, spec, &pb.PublisherAuth{Owner: spec.Admin})
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately not closed: Close on the Service must tear it down.

	done := make(chan error, 1)
	go func() {
		done <- client.Close()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("got %v want nil", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("Service.Close did not return within 4 seconds")
	}
}

// TestServiceCloseIsIdempotent pins that calling Close twice is safe and
// returns nil both times, without redoing teardown. Close closes an internal
// channel exactly once, guarded by a sync.Once rather than a bare
// select-on-quit/default, precisely so a second call cannot race the first
// into closing an already-closed channel.
//
// The two calls are concurrent deliberately: sequential calls pass even
// against the racy select-on-quit/default version, since by the time the
// second call runs the channel is visibly closed. Only overlapping calls can
// both take the branch that closes it.
func TestServiceCloseIsIdempotent(t *testing.T) {
	t.Parallel()

	broker, _, _ := newBroker(t, bps.Options{})

	var wg sync.WaitGroup
	errs := make([]error, 2)
	start := make(chan struct{})
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			<-start
			errs[i] = broker.Close()
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("close %d: got %v want nil", i, err)
		}
	}
}

// TestServiceAfterClose pins that a closed service refuses new work rather
// than handing out sessions it will never serve, and that a session torn down
// by Close refuses to publish.
func TestServiceAfterClose(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	broker, _, brokerAddr := newBroker(t, bps.Options{})
	client := newClient(t, broker, brokerAddr)

	spec, _, msg := anchorCohort(t, topic(0xc0), []byte("after close"))
	pub, err := client.Open(ctx, brokerAddr, spec, &pb.PublisherAuth{Owner: spec.Admin})
	if err != nil {
		t.Fatal(err)
	}

	if err := client.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := client.Open(ctx, brokerAddr, spec, &pb.PublisherAuth{Owner: spec.Admin}); !errors.Is(err, bps.ErrShutdown) {
		t.Fatalf("open: got %v want %v", err, bps.ErrShutdown)
	}
	if _, err := client.Subscribe(ctx, brokerAddr, swarm.NewAddress(spec.Topic), nil); !errors.Is(err, bps.ErrShutdown) {
		t.Fatalf("subscribe: got %v want %v", err, bps.ErrShutdown)
	}
	if err := pub.Publish(ctx, msg); !errors.Is(err, bps.ErrShutdown) {
		t.Fatalf("publish: got %v want %v", err, bps.ErrShutdown)
	}
}
