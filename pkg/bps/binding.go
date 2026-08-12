// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/soc"
)

// ErrNotQualified is returned when a single-owner chunk is not a legitimate
// message for the cohort under its topic binding.
var ErrNotQualified = errors.New("bps: soc does not qualify for the cohort")

// binding decides which single-owner chunks are legitimate messages for a
// cohort, and how they are deduplicated. One implementation per
// pb.TopicBinding; SWIP-60's later bindings are added here without touching
// the broker.
type binding interface {
	// qualifies reports whether s is a legitimate message for spec's cohort.
	qualifies(spec *pb.CohortSpec, s *soc.SOC) error
	// dedupKey returns the key under which s is deduplicated.
	dedupKey(s *soc.SOC) ([]byte, error)
}

// bindingFor returns the binding rules for b, or ErrUnsupportedBinding.
func bindingFor(b pb.TopicBinding) (binding, error) {
	switch b {
	case pb.TopicBinding_ANCHOR:
		return anchorBinding{}, nil
	case pb.TopicBinding_FEED_TOPIC:
		return feedTopicBinding{}, nil
	}
	return nil, fmt.Errorf("binding %s: %w", b, ErrUnsupportedBinding)
}

// anchorBinding implements SWIP-60's ANCHOR semantics: under the default
// publisher regime the topic is the full SOC address, so every message in the
// cohort shares one address. Under an explicit publisher regime (EXPLICIT_SINGLE
// or EXPLICIT_LIST) that constraint is relaxed — see qualifies — and the topic
// is a mere rendezvous: the SOC id is unconstrained and legitimacy comes from
// list membership instead. Dedup is on the wrapped content-addressed chunk —
// the guard against unsolicited republication of old SOCs — regardless of
// regime. It is sound only under the application-level requirement that
// payloads are distinct.
type anchorBinding struct{}

func (anchorBinding) qualifies(spec *pb.CohortSpec, s *soc.SOC) error {
	switch spec.GetPublishers() {
	case pb.PublisherRegime_EXPLICIT_SINGLE, pb.PublisherRegime_EXPLICIT_LIST:
		// SWIP-60: under explicit regimes the SOC id does no protocol work —
		// legitimacy is list membership, checked separately — so the topic is
		// a mere rendezvous and the address constraint does not apply.
		return nil
	}
	addr, err := s.Address()
	if err != nil {
		return fmt.Errorf("soc address: %w", err)
	}
	if !bytes.Equal(addr.Bytes(), spec.GetTopic()) {
		return fmt.Errorf("soc address %s is not the anchor: %w", addr, ErrNotQualified)
	}
	return nil
}

func (anchorBinding) dedupKey(s *soc.SOC) ([]byte, error) {
	wrapped := s.WrappedChunk()
	if wrapped == nil {
		return nil, fmt.Errorf("no wrapped chunk: %w", ErrNotQualified)
	}
	return wrapped.Address().Bytes(), nil
}

// feedTopicBinding implements SWIP-60's FEED_TOPIC semantics under explicit
// publisher regimes: id = keccak256(topic ‖ index). The index is not on the
// wire and keccak is not invertible, so the broker cannot re-derive the id;
// its enforcement point is the publisher's own node (WS bridge), which is
// handed the bare index. The binding's protocol work here is its dedup rule:
// the full chunk address, unique per (owner, index).
type feedTopicBinding struct{}

func (feedTopicBinding) qualifies(*pb.CohortSpec, *soc.SOC) error { return nil }

func (feedTopicBinding) dedupKey(s *soc.SOC) ([]byte, error) {
	addr, err := s.Address()
	if err != nil {
		return nil, fmt.Errorf("soc address: %w", err)
	}
	return addr.Bytes(), nil
}

// FeedID derives the SOC id of a feed-topic message: keccak256(topic ‖ index),
// index encoded as 8-byte big-endian, matching pkg/feeds/sequence.
func FeedID(topic []byte, index uint64) ([]byte, error) {
	buf := make([]byte, 0, len(topic)+8)
	buf = append(buf, topic...)
	buf = binary.BigEndian.AppendUint64(buf, index)
	return crypto.LegacyKeccak256(buf)
}
