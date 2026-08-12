// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bps implements the Broadcast Pub/Sub protocol specified in SWIP-60:
// a brokered, single-hop broadcast protocol carrying single-owner chunks over
// long-lived per-topic p2p streams.
//
// Every message is a single-owner chunk verified end to end by its receiver
// against the cohort spec, so a broker can withhold messages but never forge
// one. Read access is not protected to the same standard: a cohort's closed
// flag restricts admission only, and provides no confidentiality against a
// party that knows the topic and any one publisher address — both of which are
// recoverable from any message it has observed. Payloads that must stay
// private have to be encrypted by the application.
package bps

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var (
	// ErrMalformedSoc is returned when a wire Soc has fields of the wrong size
	// or a payload that is not a valid content-addressed chunk.
	ErrMalformedSoc = errors.New("bps: malformed soc")
	// ErrOwnerMismatch is returned when the owner declared on the wire is not
	// the owner recovered from the signature.
	ErrOwnerMismatch = errors.New("bps: soc owner does not match signature")
)

// SocToProto converts a single-owner chunk to its wire representation. The
// wrapped chunk's span and payload are carried separately, per SWIP-60.
// The returned message's byte slices alias the source SOC's buffers and must
// be treated as read-only; callers must not mutate them, and must copy first
// if they need to retain them beyond the source SOC's lifetime.
func SocToProto(s *soc.SOC) (*pb.Soc, error) {
	data := s.WrappedChunk().Data()
	if len(data) < swarm.SpanSize {
		return nil, fmt.Errorf("wrapped chunk too short: %w", ErrMalformedSoc)
	}
	return &pb.Soc{
		Id:        s.ID(),
		Owner:     s.OwnerAddress(),
		Signature: s.Signature(),
		Span:      data[:swarm.SpanSize],
		Payload:   data[swarm.SpanSize:],
	}, nil
}

// SocFromProto rebuilds a single-owner chunk from its wire representation and
// verifies it structurally: the owner recovered from the signature must equal
// the owner declared on the wire. A SOC returned from this function has an
// authenticated owner.
func SocFromProto(m *pb.Soc) (*soc.SOC, error) {
	switch {
	case len(m.GetId()) != swarm.HashSize:
		return nil, fmt.Errorf("id length %d: %w", len(m.GetId()), ErrMalformedSoc)
	case len(m.GetOwner()) != crypto.AddressSize:
		return nil, fmt.Errorf("owner length %d: %w", len(m.GetOwner()), ErrMalformedSoc)
	case len(m.GetSignature()) != swarm.SocSignatureSize:
		return nil, fmt.Errorf("signature length %d: %w", len(m.GetSignature()), ErrMalformedSoc)
	case len(m.GetSpan()) != swarm.SpanSize:
		return nil, fmt.Errorf("span length %d: %w", len(m.GetSpan()), ErrMalformedSoc)
	case len(m.GetPayload()) > swarm.ChunkSize:
		return nil, fmt.Errorf("payload length %d: %w", len(m.GetPayload()), ErrMalformedSoc)
	}

	wrapped := make([]byte, 0, len(m.GetSpan())+len(m.GetPayload()))
	wrapped = append(wrapped, m.GetSpan()...)
	wrapped = append(wrapped, m.GetPayload()...)
	if _, err := cac.NewWithDataSpan(wrapped); err != nil {
		return nil, fmt.Errorf("wrapped chunk: %w: %w", ErrMalformedSoc, err)
	}

	addr, err := soc.CreateAddress(m.GetId(), m.GetOwner())
	if err != nil {
		return nil, fmt.Errorf("soc address: %w: %w", ErrMalformedSoc, err)
	}

	data := make([]byte, 0, swarm.HashSize+swarm.SocSignatureSize+len(wrapped))
	data = append(data, m.GetId()...)
	data = append(data, m.GetSignature()...)
	data = append(data, wrapped...)

	s, err := soc.FromChunk(swarm.NewChunk(addr, data))
	if err != nil {
		return nil, fmt.Errorf("soc from chunk: %w: %w", ErrMalformedSoc, err)
	}
	if !bytes.Equal(s.OwnerAddress(), m.GetOwner()) {
		return nil, ErrOwnerMismatch
	}
	return s, nil
}
