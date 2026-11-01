// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/pushsync/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/topology/mock"
)

// FuzzCheckReceipt fuzzes the originator-side receipt verification that runs on
// every receipt a peer returns for a pushed chunk: signature recovery over the
// receipt address, overlay derivation from the recovered key, and the
// proximity / shallow-receipt comparison. Address, Signature, Nonce and
// StorageRadius are all peer-controlled, so the target asserts the crypto and
// arithmetic never panic on hostile input (e.g. malformed signatures, odd-length
// nonces, or a StorageRadius chosen to probe the shallow-receipt threshold).
func FuzzCheckReceipt(f *testing.F) {
	const (
		radius    uint8 = 8
		tolerance uint8 = 2
	)

	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		f.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(key)

	chunkAddr := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")
	sig, err := signer.Sign(chunkAddr.Bytes())
	if err != nil {
		f.Fatal(err)
	}
	nonce := make([]byte, swarm.HashSize)

	f.Add(chunkAddr.Bytes(), sig, nonce, uint32(0))  // recoverable, deep-ish
	f.Add(chunkAddr.Bytes(), sig, nonce, uint32(40)) // recoverable, shallow via storage radius
	f.Add(make([]byte, swarm.HashSize), []byte{}, []byte{}, uint32(0))
	f.Add([]byte{}, []byte{}, []byte{}, uint32(0))

	self := swarm.MustParseHexAddress("7000000000000000000000000000000000000000000000000000000000000000")

	f.Fuzz(func(t *testing.T, address, signature, nonce []byte, storageRadius uint32) {
		ps, _ := createPushSyncNodeWithRadius(
			t, self, defaultPrices, nil, nil, fuzzSigner, radius, tolerance,
			mock.WithClosestPeerErr(topology.ErrWantSelf),
		)

		// must not panic; any returned error is an acceptable outcome
		_ = ps.CheckReceipt(&pb.Receipt{
			Address:       address,
			Signature:     signature,
			Nonce:         nonce,
			StorageRadius: storageRadius,
		})
	})
}
