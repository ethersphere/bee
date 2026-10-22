// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzVerify fuzzes the peer-supplied Broadcast validation paths used by BPS —
// verify at a known owner, and recoverOwner at a subscriber — over every kind.
// It exercises malformed frames and valid signed chunks, asserts neither
// panics on hostile input, and that a chunk valid at an owner recovers that
// owner.
func FuzzVerify(f *testing.F) {
	priv, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		f.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(priv)
	principal, err := signer.EthereumAddress()
	if err != nil {
		f.Fatal(err)
	}

	topic := make([]byte, swarm.HashSize)
	copy(topic, "bps-fuzz-topic")
	challenge := make([]byte, ChallengeSize)
	copy(challenge, "0123456789abcdef0123456789abcdef")

	dataID, err := ID(KindData, topic, challenge, 0)
	if err != nil {
		f.Fatal(err)
	}
	payload, err := cac.New([]byte("fuzz payload"))
	if err != nil {
		f.Fatal(err)
	}
	validChunk, err := soc.New(dataID, payload).Sign(signer)
	if err != nil {
		f.Fatal(err)
	}

	authID, err := ID(KindAuth, topic, challenge, 0)
	if err != nil {
		f.Fatal(err)
	}
	authPayload, err := cac.New(nil)
	if err != nil {
		f.Fatal(err)
	}
	authChunk, err := soc.New(authID, authPayload).Sign(signer)
	if err != nil {
		f.Fatal(err)
	}
	if len(authChunk.Data()) != swarm.HashSize+swarm.SocSignatureSize+swarm.SpanSize {
		f.Fatal("auth seed not the empty auth layout")
	}

	rosterID, err := ID(KindRoster, topic, challenge, 0)
	if err != nil {
		f.Fatal(err)
	}
	rosterPayload, err := RosterPayload([][]byte{principal.Bytes()})
	if err != nil {
		f.Fatal(err)
	}
	rosterCAC, err := cac.New(rosterPayload)
	if err != nil {
		f.Fatal(err)
	}
	rosterChunk, err := soc.New(rosterID, rosterCAC).Sign(signer)
	if err != nil {
		f.Fatal(err)
	}

	eosID, err := ID(KindEOS, topic, challenge, 0)
	if err != nil {
		f.Fatal(err)
	}
	eosChunk, err := soc.New(eosID, authPayload).Sign(signer)
	if err != nil {
		f.Fatal(err)
	}

	f.Add(validChunk.Data(), uint8(pb.Kind_DATA), challenge, uint64(0), topic, principal.Bytes())
	f.Add(rosterChunk.Data(), uint8(pb.Kind_ROSTER), challenge, uint64(0), topic, principal.Bytes())
	f.Add(eosChunk.Data(), uint8(pb.Kind_EOS), challenge, uint64(0), topic, principal.Bytes())
	f.Add(authChunk.Data(), uint8(pb.Kind_AUTH), challenge, uint64(0), topic, principal.Bytes())
	f.Add([]byte{}, uint8(pb.Kind_DATA), make([]byte, ChallengeSize), uint64(0), topic, principal.Bytes())
	f.Add(make([]byte, swarm.HashSize), uint8(pb.Kind_DATA), []byte{}, uint64(0), topic[:31], principal.Bytes()[:19])

	f.Fuzz(func(t *testing.T, socData []byte, kind uint8, challenge []byte, index uint64, topic []byte, principal []byte) {
		if kind > uint8(pb.Kind_ROSTER) {
			kind = uint8(pb.Kind_KIND_UNSPECIFIED)
		}
		frame := &pb.Broadcast{Soc: socData, Kind: pb.Kind(kind), Challenge: challenge, Index: index}
		owner, recErr := recoverOwner(frame, topic)
		_, _ = rosterOf(socData)
		if err := verify(frame, topic, principal); err != nil {
			return
		}
		// a chunk valid at an owner recovers that owner
		if recErr != nil || !bytes.Equal(owner, principal) {
			t.Fatalf("valid at %x, recovered %x (%v)", principal, owner, recErr)
		}

		if len(topic) != swarm.HashSize || len(principal) != crypto.AddressSize || len(challenge) != ChallengeSize {
			return
		}
		if len(socData) >= swarm.HashSize {
			if _, err := soc.FromChunk(swarm.NewChunk(swarm.ZeroAddress, socData)); err == nil {
				_ = soc.Valid(swarm.NewChunk(swarm.ZeroAddress, socData))
			}
		}
	})
}

// FuzzRoster fuzzes the decoding of a ROSTER chunk's payload, which a peer
// supplies: it never panics, every address it returns is 20 bytes, and what
// it returns encodes and decodes to itself.
func FuzzRoster(f *testing.F) {
	off := swarm.HashSize + swarm.SocSignatureSize + swarm.SpanSize
	header := make([]byte, off)
	one, err := RosterPayload([][]byte{make([]byte, crypto.AddressSize)})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(append(append([]byte{}, header...), one...))
	f.Add(header)
	f.Add([]byte{})
	f.Add(append(append([]byte{}, header...), 0x0a, 0x02, 0x01, 0x02))
	f.Add(append(append([]byte{}, header...), 0x0a, 0xff, 0xff, 0xff, 0xff, 0x0f))

	f.Fuzz(func(t *testing.T, chunk []byte) {
		roster, err := ParseRoster(chunk)
		if err != nil {
			return
		}
		for _, a := range roster {
			if len(a) != crypto.AddressSize {
				t.Fatalf("address of %d bytes", len(a))
			}
		}
		payload, err := RosterPayload(roster)
		if err != nil {
			t.Fatal(err)
		}
		again, err := ParseRoster(append(append([]byte{}, chunk[:off]...), payload...))
		if err != nil || len(again) != len(roster) {
			t.Fatalf("round trip: %d of %d (%v)", len(again), len(roster), err)
		}
		for i := range roster {
			if !bytes.Equal(again[i], roster[i]) {
				t.Fatalf("round trip: entry %d differs", i)
			}
		}
	})
}
