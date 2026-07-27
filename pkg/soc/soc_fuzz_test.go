// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package soc_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzFromChunk fuzzes single-owner-chunk decoding, which runs on peer-supplied
// chunk data in the pushsync handler, pullsync client, and retrieval client. It
// asserts the cursor arithmetic in FromChunk never panics on truncated or
// oversized data, that an accepted SOC exposes a derivable address, and the key
// integrity property: soc.Valid reporting true implies the chunk's address
// actually binds to the SOC content (owner+id).
func FuzzFromChunk(f *testing.F) {
	priv, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		f.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(priv)

	ch, err := cac.New([]byte("fuzz payload"))
	if err != nil {
		f.Fatal(err)
	}
	id := make([]byte, swarm.HashSize)
	signed, err := soc.New(id, ch).Sign(signer)
	if err != nil {
		f.Fatal(err)
	}

	f.Add(signed.Address().Bytes(), signed.Data())       // valid SOC
	f.Add(make([]byte, swarm.HashSize), []byte{})        // empty data
	f.Add([]byte{}, make([]byte, swarm.SocMinChunkSize)) // min-size data, empty addr
	f.Add(make([]byte, swarm.HashSize), make([]byte, swarm.SocMinChunkSize-1))

	f.Fuzz(func(t *testing.T, addr, data []byte) {
		ch := swarm.NewChunk(swarm.NewAddress(addr), data)

		s, err := soc.FromChunk(ch)
		if err != nil {
			return
		}

		a, err := s.Address()
		if err != nil {
			t.Fatalf("address of an accepted soc: %v", err)
		}
		if s.WrappedChunk() == nil {
			t.Fatal("accepted soc has nil wrapped chunk")
		}
		_ = s.OwnerAddress()
		_ = s.Signature()

		// Integrity: Valid must not claim a chunk is a valid SOC unless its
		// address binds to the derived (owner+id) address.
		if soc.Valid(ch) && !ch.Address().Equal(a) {
			t.Fatal("soc.Valid reported true but chunk address does not match derived address")
		}

		// The CAC unwrap path must also tolerate any parseable SOC.
		_, _ = soc.UnwrapCAC(ch)
	})
}
