// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/bps"
	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	bpstesting "github.com/ethersphere/bee/v2/pkg/bps/testing"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// hostileBroker is a hand-driven broker: it answers the Hello with whatever
// ack says, writes frames, and then ends the stream. It speaks the wire
// protocol directly rather than going through Service, which is the only way
// to send a client something a correct broker would never send.
type hostileBroker struct {
	ack    func(hello *pb.Hello) *pb.Ack
	frames []*pb.Broadcast
}

func (h hostileBroker) handler(ctx context.Context, _ p2p.Peer, stream p2p.Stream) error {
	w, r := protobuf.NewWriterAndReader(stream)

	var hello pb.Hello
	if err := r.ReadMsgWithContext(ctx, &hello); err != nil {
		_ = stream.Reset()
		return err
	}
	if err := w.WriteMsgWithContext(ctx, h.ack(&hello)); err != nil {
		_ = stream.Reset()
		return err
	}
	for _, f := range h.frames {
		if err := w.WriteMsgWithContext(ctx, f); err != nil {
			_ = stream.Reset()
			return err
		}
	}
	// Ending the stream lets the client's read loop observe EOF and close its
	// message channel, so a test can tell "nothing was delivered" apart from
	// "nothing has been delivered yet".
	return stream.FullClose()
}

// newHostileClient returns a client service whose streamer routes to h, and
// the address to dial.
func newHostileClient(t *testing.T, h hostileBroker) (*bps.Service, swarm.Address) {
	t.Helper()

	brokerAddr := swarm.MustParseHexAddress("bada55")
	recorder := streamtest.New(
		streamtest.WithProtocols(p2p.ProtocolSpec{
			Name:    bps.ProtocolName,
			Version: bps.ProtocolVersion,
			StreamSpecs: []p2p.StreamSpec{
				{Name: bps.StreamName, Handler: h.handler},
			},
		}),
		streamtest.WithBaseAddr(brokerAddr),
	)
	client := bps.New(recorder, log.Noop, bps.Options{})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close client: %v", err)
		}
	})
	return client, brokerAddr
}

func socFrame(t *testing.T, s *soc.SOC) *pb.Broadcast {
	t.Helper()

	m, err := bps.SocToProto(s)
	if err != nil {
		t.Fatal(err)
	}
	return &pb.Broadcast{Frame: &pb.Broadcast_Soc{Soc: m}}
}

// drained collects everything a session delivers until its channel closes, or
// the deadline passes.
func drained(t *testing.T, ss *bps.Session) []*soc.SOC {
	t.Helper()

	var out []*soc.SOC
	deadline := time.After(5 * time.Second)
	for {
		select {
		case s, ok := <-ss.Messages():
			if !ok {
				return out
			}
			out = append(out, s)
		case <-deadline:
			t.Fatal("timed out waiting for the session to end")
			return nil
		}
	}
}

// TestHostileBrokerCannotForge is the protocol's headline claim: a broker can
// withhold messages but never forge one. Every message a subscriber accepts is
// verified end to end against the cohort spec, so neither of the two things a
// hostile broker can attempt under an explicit publisher regime — signing
// with a key outside the publisher set, and lying about a SOC's owner —
// reaches the consumer. (A third historical attack, sending a SOC that is not
// the cohort's anchor, is no longer one: under explicit regimes SWIP-60's
// anchor binding does not check the SOC address against the topic, because
// the id does no protocol work there — see TestAnchorMnemonicExplicitList and
// TestHostileBrokerDeliversOffAnchorMessage.)
func TestHostileBrokerCannotForge(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// The honest cohort: one publisher, ANCHOR-bound under EXPLICIT_SINGLE.
	spec, _, genuine := anchorCohort(t, topic(0xa1), []byte("genuine"))

	// (a) a message signed by a key outside the publisher set.
	outsider, _ := bpstesting.NewSigner(t)
	outsiderMsg := bpstesting.AnchorSOC(t, outsider, topic(0xa1), []byte("outsider"))

	// (b) a message whose declared owner is not the owner recovered from its
	// signature: the genuine message with the owner field swapped out.
	forgedOwner := socFrame(t, genuine)
	_, outsiderOwner := bpstesting.NewSigner(t)
	forgedOwner.GetSoc().Owner = outsiderOwner

	client, brokerAddr := newHostileClient(t, hostileBroker{
		ack: func(*pb.Hello) *pb.Ack {
			return &pb.Ack{Status: pb.Status_OK, Cohort: spec}
		},
		frames: []*pb.Broadcast{
			socFrame(t, outsiderMsg),
			forgedOwner,
		},
	})

	sub, err := client.Subscribe(ctx, brokerAddr, swarm.NewAddress(spec.Topic), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	if got := drained(t, sub); len(got) != 0 {
		t.Fatalf("hostile broker forged %d message(s) past end-to-end verification", len(got))
	}
}

// TestHostileBrokerDeliversOffAnchorMessage is the companion to the test
// above: under an explicit publisher regime, a message from the cohort's
// legitimate publisher that is not the cohort's anchor (same owner, different
// id, therefore a different SOC address) is not a forgery — it qualifies, per
// SWIP-60's relaxation of the ANCHOR binding — and does reach the consumer.
func TestHostileBrokerDeliversOffAnchorMessage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	spec, signer, _ := anchorCohort(t, topic(0xa4), []byte("genuine"))
	offAnchor := bpstesting.AnchorSOC(t, signer, topic(0xa5), []byte("off anchor"))

	client, brokerAddr := newHostileClient(t, hostileBroker{
		ack: func(*pb.Hello) *pb.Ack {
			return &pb.Ack{Status: pb.Status_OK, Cohort: spec}
		},
		frames: []*pb.Broadcast{socFrame(t, offAnchor)},
	})

	sub, err := client.Subscribe(ctx, brokerAddr, swarm.NewAddress(spec.Topic), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	got := drained(t, sub)
	if len(got) != 1 {
		t.Fatalf("delivered %d messages, want 1", len(got))
	}
	if !got[0].WrappedChunk().Address().Equal(offAnchor.WrappedChunk().Address()) {
		t.Fatal("delivered the wrong message")
	}
}

// TestHostileBrokerDeliversGenuineMessage is the control for the test above:
// the same hand-driven broker, sending a message that really is signed by the
// cohort's publisher and really is the anchor, does get through. Without it,
// the forgery test would pass just as well against a client that dropped
// everything.
func TestHostileBrokerDeliversGenuineMessage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	spec, _, genuine := anchorCohort(t, topic(0xa3), []byte("genuine"))

	client, brokerAddr := newHostileClient(t, hostileBroker{
		ack: func(*pb.Hello) *pb.Ack {
			return &pb.Ack{Status: pb.Status_OK, Cohort: spec}
		},
		frames: []*pb.Broadcast{socFrame(t, genuine)},
	})

	sub, err := client.Subscribe(ctx, brokerAddr, swarm.NewAddress(spec.Topic), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	got := drained(t, sub)
	if len(got) != 1 {
		t.Fatalf("delivered %d messages, want 1", len(got))
	}
	if !got[0].WrappedChunk().Address().Equal(genuine.WrappedChunk().Address()) {
		t.Fatal("delivered the wrong message")
	}
}

// TestOpenRefusesTamperedSpecEcho pins that a client that asked for a specific
// cohort keeps the spec it asked for: the echoed spec is compared field for
// field and a broker that substitutes one is refused, rather than having its
// version adopted as the rule every later message is verified against.
func TestOpenRefusesTamperedSpecEcho(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	spec, _, _ := anchorCohort(t, topic(0xa4), []byte("tampered echo"))

	tampered := &pb.CohortSpec{
		Topic:      spec.Topic,
		Binding:    spec.Binding,
		Publishers: spec.Publishers,
		Admin:      spec.Admin,
		Closed:     true,
	}

	client, brokerAddr := newHostileClient(t, hostileBroker{
		ack: func(*pb.Hello) *pb.Ack {
			return &pb.Ack{Status: pb.Status_OK, Cohort: tampered}
		},
	})

	_, err := client.Open(ctx, brokerAddr, spec, &pb.PublisherAuth{Owner: spec.Admin})
	if !errors.Is(err, bps.ErrSpecMismatch) {
		t.Fatalf("got %v want %v", err, bps.ErrSpecMismatch)
	}

	// Subscribe cannot make the same check: it has nothing to compare the
	// echo against, and so accepts it. This is the documented asymmetry, not
	// an oversight — under ANCHOR the topic pins the owner regardless of what
	// the spec claims.
	sub, err := client.Subscribe(ctx, brokerAddr, swarm.NewAddress(spec.Topic), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	if !sub.Spec().GetClosed() {
		t.Fatal("expected Subscribe to adopt the broker's echoed spec")
	}
}

// TestSessionSkipsUnknownFrame pins the forward-compatibility promise: a
// Broadcast whose oneof is unset — a control frame reserved for bps-multihop —
// is skipped rather than ending the session, so a later revision can add
// frames without a version bump.
func TestSessionSkipsUnknownFrame(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	spec, _, genuine := anchorCohort(t, topic(0xa5), []byte("after the unknown frame"))

	client, brokerAddr := newHostileClient(t, hostileBroker{
		ack: func(*pb.Hello) *pb.Ack {
			return &pb.Ack{Status: pb.Status_OK, Cohort: spec}
		},
		frames: []*pb.Broadcast{
			{}, // reserved multihop control frame
			socFrame(t, genuine),
		},
	})

	sub, err := client.Subscribe(ctx, brokerAddr, swarm.NewAddress(spec.Topic), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	got := drained(t, sub)
	if len(got) != 1 {
		t.Fatalf("delivered %d messages, want 1 — the unknown frame was not skipped", len(got))
	}
	if !got[0].WrappedChunk().Address().Equal(genuine.WrappedChunk().Address()) {
		t.Fatal("delivered the wrong message")
	}
}
