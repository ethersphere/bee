// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bps"
	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	bpstesting "github.com/ethersphere/bee/v2/pkg/bps/testing"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

func TestAnchorBindingQualifies(t *testing.T) {
	t.Parallel()

	signer, owner := bpstesting.NewSigner(t)
	id := topic(0x11)
	s := bpstesting.AnchorSOC(t, signer, id, []byte("anchored"))
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

	b, err := bps.BindingFor(pb.TopicBinding_ANCHOR)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Qualifies(spec, s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Under explicit regimes the SOC id does no protocol work — legitimacy is
	// list membership, checked separately — so a SOC under a different id
	// still qualifies for this topic.
	other := bpstesting.AnchorSOC(t, signer, topic(0x12), []byte("anchored"))
	if err := b.Qualifies(spec, other); err != nil {
		t.Fatalf("unexpected error under explicit regime: %v", err)
	}
}

// TestAnchorBindingStrictAddressCheck pins the address-equals-topic check
// that still applies outside the explicit publisher regimes. No such regime
// is implemented yet (SWIP-60's future IMPLICIT regime), so this exercises
// the binding directly rather than through ValidateSpec.
func TestAnchorBindingStrictAddressCheck(t *testing.T) {
	t.Parallel()

	signer, _ := bpstesting.NewSigner(t)
	id := topic(0x13)
	s := bpstesting.AnchorSOC(t, signer, id, []byte("anchored"))
	anchor, err := s.Address()
	if err != nil {
		t.Fatal(err)
	}

	spec := &pb.CohortSpec{
		Topic:      anchor.Bytes(),
		Binding:    pb.TopicBinding_ANCHOR,
		Publishers: pb.PublisherRegime_IMPLICIT,
	}

	b, err := bps.BindingFor(pb.TopicBinding_ANCHOR)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Qualifies(spec, s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A SOC under a different id has a different address, so it does not
	// belong to this anchor cohort.
	other := bpstesting.AnchorSOC(t, signer, topic(0x14), []byte("anchored"))
	if err := b.Qualifies(spec, other); !errors.Is(err, bps.ErrNotQualified) {
		t.Fatalf("got %v want %v", err, bps.ErrNotQualified)
	}
}

// TestAnchorMnemonicExplicitList pins SWIP-60's relaxation of the ANCHOR
// binding under explicit publisher regimes: a mnemonic topic shared by
// multiple listed publishers can never satisfy the SOC-address-equals-topic
// check, so that check must not apply when publishers are explicit.
func TestAnchorMnemonicExplicitList(t *testing.T) {
	t.Parallel()

	topic := swarm.NewAddress(testutil.RandBytes(t, swarm.HashSize))
	sA, ownerA := bpstesting.NewSigner(t)
	sB, ownerB := bpstesting.NewSigner(t)

	spec := &pb.CohortSpec{
		Topic:         topic.Bytes(),
		Binding:       pb.TopicBinding_ANCHOR,
		Publishers:    pb.PublisherRegime_EXPLICIT_LIST,
		Admin:         ownerA,
		PublisherList: [][]byte{ownerB},
	}
	if err := bps.ValidateSpec(spec); err != nil {
		t.Fatal(err)
	}

	b, err := bps.BindingFor(pb.TopicBinding_ANCHOR)
	if err != nil {
		t.Fatal(err)
	}

	msgA := bpstesting.AnchorSOC(t, sA, topic.Bytes(), []byte("seat A says hi"))
	msgB := bpstesting.AnchorSOC(t, sB, topic.Bytes(), []byte("seat B says hi"))

	for _, m := range []*soc.SOC{msgA, msgB} {
		if err := b.Qualifies(spec, m); err != nil {
			t.Fatalf("mnemonic-anchor message must qualify: %v", err)
		}
	}

	// Dedup keys of two distinct payloads must differ...
	ka, err := b.DedupKey(msgA)
	if err != nil {
		t.Fatal(err)
	}
	kb, err := b.DedupKey(msgB)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(ka, kb) {
		t.Fatal("expected different dedup keys for different payloads")
	}

	// ...but two SOCs wrapping the same CAC must collide on dedup key.
	msgA2 := bpstesting.AnchorSOC(t, sB, topic.Bytes(), []byte("seat A says hi"))
	if err := b.Qualifies(spec, msgA2); err != nil {
		t.Fatalf("mnemonic-anchor message must qualify: %v", err)
	}
	ka2, err := b.DedupKey(msgA2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ka, ka2) {
		t.Fatal("expected identical dedup keys for identical wrapped payloads")
	}
}

func TestAnchorBindingDedupKey(t *testing.T) {
	t.Parallel()

	signer, _ := bpstesting.NewSigner(t)
	b, err := bps.BindingFor(pb.TopicBinding_ANCHOR)
	if err != nil {
		t.Fatal(err)
	}

	// Same payload under different ids: same wrapped CAC, so same dedup key.
	a := bpstesting.AnchorSOC(t, signer, topic(0x21), []byte("same payload"))
	c := bpstesting.AnchorSOC(t, signer, topic(0x22), []byte("same payload"))
	d := bpstesting.AnchorSOC(t, signer, topic(0x21), []byte("other payload"))

	ka, err := b.DedupKey(a)
	if err != nil {
		t.Fatal(err)
	}
	kc, err := b.DedupKey(c)
	if err != nil {
		t.Fatal(err)
	}
	kd, err := b.DedupKey(d)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ka, kc) {
		t.Fatal("expected identical dedup keys for identical payloads")
	}
	if bytes.Equal(ka, kd) {
		t.Fatal("expected different dedup keys for different payloads")
	}
	if len(ka) != swarm.HashSize {
		t.Fatalf("dedup key length: got %d want %d", len(ka), swarm.HashSize)
	}
}

func TestBindingForUnsupported(t *testing.T) {
	t.Parallel()

	for _, b := range []pb.TopicBinding{
		pb.TopicBinding_TOPIC_BINDING_UNSPECIFIED,
		pb.TopicBinding_SOC_ID,
		pb.TopicBinding_OWNER,
	} {
		if _, err := bps.BindingFor(b); !errors.Is(err, bps.ErrUnsupportedBinding) {
			t.Fatalf("binding %s: got %v want %v", b, err, bps.ErrUnsupportedBinding)
		}
	}
}

// TestFeedTopicBinding pins SWIP-60's FEED_TOPIC semantics: a spec using it
// validates under an explicit publisher regime, FeedSOC derives its id via
// FeedID, and dedup keys are distinct across indices but collide for
// identical SOCs.
func TestFeedTopicBinding(t *testing.T) {
	t.Parallel()

	feedTopic := testutil.RandBytes(t, swarm.HashSize)
	signer, owner := bpstesting.NewSigner(t)
	spec := &pb.CohortSpec{
		Topic:      feedTopic,
		Binding:    pb.TopicBinding_FEED_TOPIC,
		Publishers: pb.PublisherRegime_EXPLICIT_SINGLE,
		Admin:      owner,
	}
	if err := bps.ValidateSpec(spec); err != nil {
		t.Fatalf("feed-topic explicit-single spec must validate: %v", err)
	}

	m0 := bpstesting.FeedSOC(t, signer, feedTopic, 0, []byte("frame 0"))
	m1 := bpstesting.FeedSOC(t, signer, feedTopic, 1, []byte("frame 1"))

	id0, err := bps.FeedID(feedTopic, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(m0.ID(), id0) {
		t.Fatal("FeedSOC id must be FeedID(topic, index)")
	}

	b, err := bps.BindingFor(pb.TopicBinding_FEED_TOPIC)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Qualifies(spec, m0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	k0, err := b.DedupKey(m0)
	if err != nil {
		t.Fatal(err)
	}
	k1, err := b.DedupKey(m1)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(k0, k1) {
		t.Fatal("distinct indices must not collide on dedup key")
	}

	m0Again := bpstesting.FeedSOC(t, signer, feedTopic, 0, []byte("frame 0"))
	k0Again, err := b.DedupKey(m0Again)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(k0, k0Again) {
		t.Fatal("identical soc must collide on dedup key")
	}
}

// TestFeedIDIndependentOracle checks FeedID against an oracle assembled by
// hand in this test, independently of FeedID's own byte-appending logic:
// the index is written out as eight literal big-endian bytes rather than via
// binary.BigEndian.AppendUint64, so a wrong endianness, operand order, or
// hash function in FeedID would be caught rather than silently agreeing with
// itself (unlike comparing FeedID against FeedSOC, which internally calls
// FeedID and so shares its code path).
func TestFeedIDIndependentOracle(t *testing.T) {
	t.Parallel()

	feedTopic := testutil.RandBytes(t, swarm.HashSize)

	// index = 42, hand-written as 8 big-endian bytes: 0x00,...,0x00,0x2a.
	want := append(append([]byte{}, feedTopic...), 0, 0, 0, 0, 0, 0, 0, 42)
	oracle, err := crypto.LegacyKeccak256(want)
	if err != nil {
		t.Fatal(err)
	}

	got, err := bps.FeedID(feedTopic, 42)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, oracle) {
		t.Fatalf("FeedID(topic, 42) = %x, want %x (independent oracle)", got, oracle)
	}

	// A second, independently-chosen vector at a different index, to guard
	// against an oracle that happens to match only by coincidence.
	want2 := append(append([]byte{}, feedTopic...), 0, 0, 0, 0, 0, 0, 1, 0)
	oracle2, err := crypto.LegacyKeccak256(want2)
	if err != nil {
		t.Fatal(err)
	}
	got2, err := bps.FeedID(feedTopic, 256)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got2, oracle2) {
		t.Fatalf("FeedID(topic, 256) = %x, want %x (independent oracle)", got2, oracle2)
	}
	if bytes.Equal(got, got2) {
		t.Fatal("distinct indices must not produce the same id")
	}
}

func TestAuthorizePublisher(t *testing.T) {
	t.Parallel()

	admin, second, stranger := addr(0x01), addr(0x02), addr(0xff)

	list := &pb.CohortSpec{
		Topic:         topic(0xaa),
		Binding:       pb.TopicBinding_ANCHOR,
		Publishers:    pb.PublisherRegime_EXPLICIT_LIST,
		Admin:         admin,
		PublisherList: [][]byte{second},
	}
	single := &pb.CohortSpec{
		Topic:      topic(0xaa),
		Binding:    pb.TopicBinding_ANCHOR,
		Publishers: pb.PublisherRegime_EXPLICIT_SINGLE,
		Admin:      admin,
	}

	for _, tc := range []struct {
		name  string
		spec  *pb.CohortSpec
		owner []byte
		want  error
	}{
		{name: "list admin", spec: list, owner: admin},
		{name: "list member", spec: list, owner: second},
		{name: "list stranger", spec: list, owner: stranger, want: bps.ErrNotPublisher},
		{name: "single admin", spec: single, owner: admin},
		{name: "single non-admin", spec: single, owner: second, want: bps.ErrNotPublisher},
		{name: "empty owner", spec: list, owner: nil, want: bps.ErrNotPublisher},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := bps.AuthorizePublisher(tc.spec, tc.owner)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
}
