// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bps"
	bpstesting "github.com/ethersphere/bee/v2/pkg/bps/testing"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestSocProtoRoundTrip(t *testing.T) {
	t.Parallel()

	signer, owner := bpstesting.NewSigner(t)
	id := bytes.Repeat([]byte{0x01}, swarm.HashSize)
	s := bpstesting.AnchorSOC(t, signer, id, []byte("hello bps"))

	m, err := bps.SocToProto(s)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(m.Owner, owner) {
		t.Fatalf("owner: got %x want %x", m.Owner, owner)
	}
	if len(m.Span) != swarm.SpanSize {
		t.Fatalf("span length: got %d want %d", len(m.Span), swarm.SpanSize)
	}

	got, err := bps.SocFromProto(m)
	if err != nil {
		t.Fatal(err)
	}

	wantAddr, err := s.Address()
	if err != nil {
		t.Fatal(err)
	}
	gotAddr, err := got.Address()
	if err != nil {
		t.Fatal(err)
	}
	if !gotAddr.Equal(wantAddr) {
		t.Fatalf("address: got %s want %s", gotAddr, wantAddr)
	}
	if !bytes.Equal(got.WrappedChunk().Data(), s.WrappedChunk().Data()) {
		t.Fatal("wrapped chunk data mismatch")
	}
}

func TestSocFromProtoRejectsForgedOwner(t *testing.T) {
	t.Parallel()

	signer, _ := bpstesting.NewSigner(t)
	_, other := bpstesting.NewSigner(t)
	id := bytes.Repeat([]byte{0x02}, swarm.HashSize)
	s := bpstesting.AnchorSOC(t, signer, id, []byte("forged"))

	m, err := bps.SocToProto(s)
	if err != nil {
		t.Fatal(err)
	}
	m.Owner = other

	if _, err := bps.SocFromProto(m); !errors.Is(err, bps.ErrOwnerMismatch) {
		t.Fatalf("got %v want %v", err, bps.ErrOwnerMismatch)
	}
}

func TestSocFromProtoRejectsMalformed(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		mut  func(m *pbSocFields)
	}{
		{name: "short id", mut: func(m *pbSocFields) { m.id = m.id[:16] }},
		{name: "short owner", mut: func(m *pbSocFields) { m.owner = m.owner[:10] }},
		{name: "short signature", mut: func(m *pbSocFields) { m.signature = m.signature[:32] }},
		{name: "short span", mut: func(m *pbSocFields) { m.span = m.span[:4] }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			signer, _ := bpstesting.NewSigner(t)
			id := bytes.Repeat([]byte{0x03}, swarm.HashSize)
			s := bpstesting.AnchorSOC(t, signer, id, []byte("malformed"))
			m, err := bps.SocToProto(s)
			if err != nil {
				t.Fatal(err)
			}

			f := &pbSocFields{id: m.Id, owner: m.Owner, signature: m.Signature, span: m.Span}
			tc.mut(f)
			m.Id, m.Owner, m.Signature, m.Span = f.id, f.owner, f.signature, f.span

			if _, err := bps.SocFromProto(m); !errors.Is(err, bps.ErrMalformedSoc) {
				t.Fatalf("got %v want %v", err, bps.ErrMalformedSoc)
			}
		})
	}
}

type pbSocFields struct {
	id        []byte
	owner     []byte
	signature []byte
	span      []byte
}
