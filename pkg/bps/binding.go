// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// claimIDPrefix domain-separates the cohort's claim SOC id from the
// principal's other single owner chunks.
var claimIDPrefix = []byte("bps-claim")

// errUnsupportedBinding is returned for a Join naming a topic binding this
// node does not implement.
var errUnsupportedBinding = errors.New("bps: unsupported topic binding")

// errClaimNotQualified is returned when a claim SOC does not satisfy its
// cohort's topic binding.
var errClaimNotQualified = errors.New("bps: claim does not qualify")

// binding decides how a cohort's publisher claim is verified. The topic
// binding named in Join picks the implementation; feed is the only one for now.
type binding interface {
	// claimAddress returns the SOC address a valid claim must have for the
	// cohort's topic and governing principal.
	claimAddress(topic, principal []byte) (swarm.Address, error)
	// verifyClaim checks the decoded claim's owner and signed payload. It
	// returns nil when the claim promotes the stream to publisher.
	verifyClaim(s *soc.SOC, principal, challenge, broker, topic []byte) error
}

// bindingFor returns the binding rules for b, or errUnsupportedBinding.
func bindingFor(b pb.TopicBinding) (binding, error) {
	switch b {
	case pb.TopicBinding_BINDING_FEED:
		return feedBinding{}, nil
	}
	return nil, fmt.Errorf("%s: %w", b, errUnsupportedBinding)
}

// feedBinding verifies the publisher claim for a feed topic. The claim is a
// SOC with id keccak256("bps-claim" | topic), owned by the governing principal,
// whose wrapped payload signs the broker overlay and feed topic together with
// the per-member challenge.
type feedBinding struct{}

func (feedBinding) claimAddress(topic, principal []byte) (swarm.Address, error) {
	id, err := feedClaimID(topic)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	return soc.CreateAddress(id, principal)
}

func (feedBinding) verifyClaim(s *soc.SOC, principal, challenge, broker, topic []byte) error {
	if !bytes.Equal(s.OwnerAddress(), principal) {
		return fmt.Errorf("claim owner is not the principal: %w", errClaimNotQualified)
	}
	innerProof := make([]byte, 0, len(challenge)+len(broker)+len(topic))
	innerProof = append(innerProof, challenge...)
	innerProof = append(innerProof, broker...)
	innerProof = append(innerProof, topic...)
	if !bytes.Equal(s.WrappedChunk().Data()[swarm.SpanSize:], innerProof) {
		return fmt.Errorf("claim payload mismatch: %w", errClaimNotQualified)
	}
	return nil
}

// feedClaimID derives the claim SOC id for a feed topic.
func feedClaimID(topic []byte) ([]byte, error) {
	return crypto.LegacyKeccak256(append(append([]byte{}, claimIDPrefix...), topic...))
}
