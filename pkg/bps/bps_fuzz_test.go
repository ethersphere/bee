// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzVerify fuzzes the peer-supplied Broadcast validation path used by BPS.
// It exercises malformed frames and valid signed chunks and asserts the verifier
// never panics on hostile input.
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

	f.Add(validChunk.Data(), uint8(pb.Kind_DATA), challenge, uint64(0), topic, principal.Bytes())
	f.Add(authChunk.Data(), uint8(pb.Kind_AUTH), challenge, uint64(0), topic, principal.Bytes())
	f.Add([]byte{}, uint8(pb.Kind_DATA), make([]byte, ChallengeSize), uint64(0), topic, principal.Bytes())
	f.Add(make([]byte, swarm.HashSize), uint8(pb.Kind_DATA), []byte{}, uint64(0), topic[:31], principal.Bytes()[:19])

	f.Fuzz(func(t *testing.T, socData []byte, kind uint8, challenge []byte, index uint64, topic []byte, principal []byte) {
		if kind > uint8(pb.Kind_AUTH) {
			kind = uint8(pb.Kind_KIND_UNSPECIFIED)
		}
		if err := verify(&pb.Broadcast{Soc: socData, Kind: pb.Kind(kind), Challenge: challenge, Index: index}, topic, principal); err != nil {
			return
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
