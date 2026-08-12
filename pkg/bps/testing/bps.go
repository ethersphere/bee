// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package testing provides fixture builders for BPS protocol tests.
package testing

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bps"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/soc"
)

// NewSigner returns a random signer and the 20-byte ethereum address of its key.
func NewSigner(t *testing.T) (crypto.Signer, []byte) {
	t.Helper()

	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(key)
	owner, err := signer.EthereumAddress()
	if err != nil {
		t.Fatal(err)
	}
	return signer, owner.Bytes()
}

// AnchorSOC builds a signed single-owner chunk wrapping payload under id.
func AnchorSOC(t *testing.T, signer crypto.Signer, id, payload []byte) *soc.SOC {
	t.Helper()

	ch, err := cac.New(payload)
	if err != nil {
		t.Fatal(err)
	}
	signed, err := soc.New(id, ch).Sign(signer)
	if err != nil {
		t.Fatal(err)
	}
	s, err := soc.FromChunk(signed)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// FeedSOC builds a signed single-owner chunk wrapping payload under the
// feed-topic id derived from topic and index.
func FeedSOC(t *testing.T, signer crypto.Signer, topic []byte, index uint64, payload []byte) *soc.SOC {
	t.Helper()

	id, err := bps.FeedID(topic, index)
	if err != nil {
		t.Fatal(err)
	}
	return AnchorSOC(t, signer, id, payload)
}
