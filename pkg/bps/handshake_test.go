// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps_test

import (
	"context"
	"sync"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bps"
	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// newBroker returns a broker service and a streamer that routes to it.
func newBroker(t *testing.T, o bps.Options) (*bps.Service, p2p.Streamer, swarm.Address) {
	t.Helper()

	brokerAddr := swarm.MustParseHexAddress("ca11ab1e")
	broker := bps.New(nil, log.Noop, o)
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Fatal(err)
		}
	})
	recorder := streamtest.New(
		streamtest.WithProtocols(broker.Protocol()),
		streamtest.WithBaseAddr(brokerAddr),
	)
	return broker, recorder, brokerAddr
}

// handshake writes one Hello and reads the Ack, on a fresh stream.
func handshake(t *testing.T, streamer p2p.Streamer, peer swarm.Address, hello *pb.Hello) *pb.Ack {
	t.Helper()

	ctx := context.Background()
	stream, err := streamer.NewStream(ctx, peer, nil, bps.ProtocolName, bps.ProtocolVersion, bps.StreamName)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stream.Reset() })

	w, r := protobuf.NewWriterAndReader(stream)
	if err := w.WriteMsgWithContext(ctx, hello); err != nil {
		t.Fatal(err)
	}
	var ack pb.Ack
	if err := r.ReadMsgWithContext(ctx, &ack); err != nil {
		t.Fatal(err)
	}
	return &ack
}

// rawStream opens a stream to the broker, completes the handshake by hand and
// hands the still-open stream back, so a test can go on speaking the wire
// protocol itself — sending frames a well-behaved Session would refuse to
// send, or refusing to read what the broker sends back.
func rawStream(t *testing.T, streamer p2p.Streamer, peer swarm.Address, hello *pb.Hello) (p2p.Stream, protobuf.Writer, protobuf.Reader, *pb.Ack) {
	t.Helper()

	ctx := context.Background()
	stream, err := streamer.NewStream(ctx, peer, nil, bps.ProtocolName, bps.ProtocolVersion, bps.StreamName)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stream.Reset() })

	w, r := protobuf.NewWriterAndReader(stream)
	if err := w.WriteMsgWithContext(ctx, hello); err != nil {
		t.Fatal(err)
	}
	var ack pb.Ack
	if err := r.ReadMsgWithContext(ctx, &ack); err != nil {
		t.Fatal(err)
	}
	return stream, w, r, &ack
}

func openHello(spec *pb.CohortSpec, auth *pb.PublisherAuth) *pb.Hello {
	return &pb.Hello{Handshake: &pb.Hello_Open{Open: &pb.Open{Cohort: spec, Auth: auth}}}
}

func subscribeHello(topic []byte, auth *pb.PublisherAuth) *pb.Hello {
	return &pb.Hello{Handshake: &pb.Hello_Subscribe{Subscribe: &pb.Subscribe{Topic: topic, Auth: auth}}}
}

func TestHandshakeOpen(t *testing.T) {
	t.Parallel()

	broker, streamer, brokerAddr := newBroker(t, bps.Options{})
	spec := validSpec()

	ack := handshake(t, streamer, brokerAddr, openHello(spec, &pb.PublisherAuth{Owner: spec.Admin}))
	if ack.Status != pb.Status_OK {
		t.Fatalf("status: got %s want OK", ack.Status)
	}
	if !bps.SpecEqual(ack.Cohort, spec) {
		t.Fatal("Ack did not echo the cohort spec")
	}
	if got := broker.Topics(); len(got) != 1 || !got[0].Equal(swarm.NewAddress(spec.Topic)) {
		t.Fatalf("topics: got %v", got)
	}
}

func TestHandshakeOpenIsIdempotent(t *testing.T) {
	t.Parallel()

	_, streamer, brokerAddr := newBroker(t, bps.Options{})
	spec := validSpec()
	auth := &pb.PublisherAuth{Owner: spec.Admin}

	if ack := handshake(t, streamer, brokerAddr, openHello(spec, auth)); ack.Status != pb.Status_OK {
		t.Fatalf("first open: got %s want OK", ack.Status)
	}
	// An identical spec is equivalent to Subscribe.
	if ack := handshake(t, streamer, brokerAddr, openHello(validSpec(), auth)); ack.Status != pb.Status_OK {
		t.Fatalf("idempotent open: got %s want OK", ack.Status)
	}

	mismatched := validSpec()
	mismatched.Closed = true
	if ack := handshake(t, streamer, brokerAddr, openHello(mismatched, auth)); ack.Status != pb.Status_REJECTED {
		t.Fatalf("mismatched open: got %s want REJECTED", ack.Status)
	}
}

func TestHandshakeSubscribe(t *testing.T) {
	t.Parallel()

	_, streamer, brokerAddr := newBroker(t, bps.Options{})
	spec := validSpec()

	if ack := handshake(t, streamer, brokerAddr, subscribeHello(spec.Topic, nil)); ack.Status != pb.Status_UNKNOWN_TOPIC {
		t.Fatalf("unknown topic: got %s want UNKNOWN_TOPIC", ack.Status)
	}

	if ack := handshake(t, streamer, brokerAddr, openHello(spec, &pb.PublisherAuth{Owner: spec.Admin})); ack.Status != pb.Status_OK {
		t.Fatal("open rejected")
	}

	ack := handshake(t, streamer, brokerAddr, subscribeHello(spec.Topic, nil))
	if ack.Status != pb.Status_OK {
		t.Fatalf("subscribe: got %s want OK", ack.Status)
	}
	if !bps.SpecEqual(ack.Cohort, spec) {
		t.Fatal("Ack did not echo the cohort spec to the subscriber")
	}
}

func TestHandshakeRejections(t *testing.T) {
	t.Parallel()

	closedSpec := validSpec()
	closedSpec.Closed = true

	t.Run("closed cohort refuses a non-publisher", func(t *testing.T) {
		t.Parallel()

		_, streamer, brokerAddr := newBroker(t, bps.Options{})
		if ack := handshake(t, streamer, brokerAddr, openHello(closedSpec, &pb.PublisherAuth{Owner: closedSpec.Admin})); ack.Status != pb.Status_OK {
			t.Fatal("open rejected")
		}
		if ack := handshake(t, streamer, brokerAddr, subscribeHello(closedSpec.Topic, nil)); ack.Status != pb.Status_REJECTED {
			t.Fatalf("got %s want REJECTED", ack.Status)
		}
	})

	t.Run("publisher outside the genesis list", func(t *testing.T) {
		t.Parallel()

		_, streamer, brokerAddr := newBroker(t, bps.Options{})
		spec := validSpec()
		if ack := handshake(t, streamer, brokerAddr, openHello(spec, &pb.PublisherAuth{Owner: spec.Admin})); ack.Status != pb.Status_OK {
			t.Fatal("open rejected")
		}
		if ack := handshake(t, streamer, brokerAddr, subscribeHello(spec.Topic, &pb.PublisherAuth{Owner: addr(0xee)})); ack.Status != pb.Status_REJECTED {
			t.Fatalf("got %s want REJECTED", ack.Status)
		}
	})

	t.Run("invalid spec", func(t *testing.T) {
		t.Parallel()

		_, streamer, brokerAddr := newBroker(t, bps.Options{})
		bad := validSpec()
		bad.Binding = pb.TopicBinding_TOPIC_BINDING_UNSPECIFIED
		if ack := handshake(t, streamer, brokerAddr, openHello(bad, nil)); ack.Status != pb.Status_REJECTED {
			t.Fatalf("got %s want REJECTED", ack.Status)
		}
	})

	t.Run("empty hello", func(t *testing.T) {
		t.Parallel()

		_, streamer, brokerAddr := newBroker(t, bps.Options{})
		if ack := handshake(t, streamer, brokerAddr, &pb.Hello{}); ack.Status != pb.Status_REJECTED {
			t.Fatalf("got %s want REJECTED", ack.Status)
		}
	})
}

func TestHandshakeCapacity(t *testing.T) {
	t.Parallel()

	_, streamer, brokerAddr := newBroker(t, bps.Options{Capacity: 2})
	spec := validSpec()
	auth := &pb.PublisherAuth{Owner: spec.Admin}

	// The opener takes the first slot.
	if ack := handshake(t, streamer, brokerAddr, openHello(spec, auth)); ack.Status != pb.Status_OK {
		t.Fatalf("open: got %s want OK", ack.Status)
	}
	// The second peer takes the last slot.
	if ack := handshake(t, streamer, brokerAddr, subscribeHello(spec.Topic, nil)); ack.Status != pb.Status_OK {
		t.Fatalf("second peer: got %s want OK", ack.Status)
	}
	// The third is refused with FULL — and nothing else. A singlehop broker
	// never refers.
	ack := handshake(t, streamer, brokerAddr, subscribeHello(spec.Topic, nil))
	if ack.Status != pb.Status_FULL {
		t.Fatalf("third peer: got %s want FULL", ack.Status)
	}
	if ack.Cohort != nil {
		t.Fatal("a refusal must not echo the cohort spec")
	}

	// An Open at capacity is refused the same way.
	if ack := handshake(t, streamer, brokerAddr, openHello(validSpec(), auth)); ack.Status != pb.Status_FULL {
		t.Fatalf("open at capacity: got %s want FULL", ack.Status)
	}
}

// TestHandshakeCohortLimit pins the registry cap: cohorts are never reclaimed,
// so the number a peer can make a broker fix has to be bounded. Joining a
// cohort that already exists is deliberately unaffected.
func TestHandshakeCohortLimit(t *testing.T) {
	t.Parallel()

	broker, streamer, brokerAddr := newBroker(t, bps.Options{MaxCohorts: 1})
	first := validSpec()
	auth := &pb.PublisherAuth{Owner: first.Admin}

	if ack := handshake(t, streamer, brokerAddr, openHello(first, auth)); ack.Status != pb.Status_OK {
		t.Fatalf("first open: got %s want OK", ack.Status)
	}

	second := validSpec()
	second.Topic = topic(0xbb)
	if ack := handshake(t, streamer, brokerAddr, openHello(second, auth)); ack.Status != pb.Status_FULL {
		t.Fatalf("open beyond the limit: got %s want FULL", ack.Status)
	}
	if got := broker.Topics(); len(got) != 1 {
		t.Fatalf("topics: got %d want 1 — a refused Open registered a cohort", len(got))
	}

	// The existing cohort still admits peers, by Open and by Subscribe alike.
	if ack := handshake(t, streamer, brokerAddr, openHello(validSpec(), auth)); ack.Status != pb.Status_OK {
		t.Fatalf("idempotent open at the limit: got %s want OK", ack.Status)
	}
	if ack := handshake(t, streamer, brokerAddr, subscribeHello(first.Topic, nil)); ack.Status != pb.Status_OK {
		t.Fatalf("subscribe at the limit: got %s want OK", ack.Status)
	}
}

// TestHandshakeCapacityConcurrent fires more handshakes at a cohort than its
// capacity allows, all at once, and asserts exactly capacity-1 of them are
// admitted (one slot is already taken by the synchronous opener below). If
// the capacity check and the peer's registration are not atomic under the
// same lock, concurrent handshakes can all observe room for the last slot
// and all be admitted, over-subscribing the cohort.
func TestHandshakeCapacityConcurrent(t *testing.T) {
	t.Parallel()

	const capacity = 4
	const attempts = 20

	_, streamer, brokerAddr := newBroker(t, bps.Options{Capacity: capacity})
	spec := validSpec()
	auth := &pb.PublisherAuth{Owner: spec.Admin}

	// The opener takes the first slot synchronously, so the concurrent
	// subscribers below race for exactly the remaining capacity-1 slots.
	if ack := handshake(t, streamer, brokerAddr, openHello(spec, auth)); ack.Status != pb.Status_OK {
		t.Fatalf("open: got %s want OK", ack.Status)
	}

	var wg sync.WaitGroup
	statuses := make([]pb.Status, attempts)
	for i := range attempts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			ctx := context.Background()
			stream, err := streamer.NewStream(ctx, brokerAddr, nil, bps.ProtocolName, bps.ProtocolVersion, bps.StreamName)
			if err != nil {
				t.Errorf("new stream: %v", err)
				return
			}
			defer func() { _ = stream.Reset() }()

			w, r := protobuf.NewWriterAndReader(stream)
			if err := w.WriteMsgWithContext(ctx, subscribeHello(spec.Topic, nil)); err != nil {
				t.Errorf("write hello: %v", err)
				return
			}
			var ack pb.Ack
			if err := r.ReadMsgWithContext(ctx, &ack); err != nil {
				t.Errorf("read ack: %v", err)
				return
			}
			statuses[i] = ack.Status
		}(i)
	}
	wg.Wait()

	var admitted, full int
	for _, status := range statuses {
		switch status {
		case pb.Status_OK:
			admitted++
		case pb.Status_FULL:
			full++
		default:
			t.Fatalf("unexpected status %s", status)
		}
	}
	if want := capacity - 1; admitted != want {
		t.Fatalf("admitted: got %d want %d — capacity was not enforced atomically", admitted, want)
	}
	if want := attempts - (capacity - 1); full != want {
		t.Fatalf("refused: got %d want %d", full, want)
	}
}
