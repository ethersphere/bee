// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2ptest

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology/lightnode"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

// NewLibp2pService creates a new libp2p service for testing purposes.
func NewLibp2pService(t *testing.T, networkID uint64, logger log.Logger) (*libp2p.Service, swarm.Address) {
	t.Helper()

	swarmKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	nonce := common.HexToHash("0x1").Bytes()

	overlay, err := crypto.NewOverlayAddress(swarmKey.PublicKey, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}

	addr := ":0"

	statestore := mock.NewStateStore()
	ab := addressbook.New(statestore)

	libp2pKey, err := crypto.GenerateSecp256r1Key()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	lightNodes := lightnode.NewContainer(overlay)

	opts := libp2p.Options{
		PrivateKey: libp2pKey,
		Nonce:      nonce,
		FullNode:   true,
		NATAddr:    "127.0.0.1:0", // Disable default NAT manager
	}

	s, err := libp2p.New(ctx, crypto.NewDefaultSigner(swarmKey), networkID, overlay, addr, ab, statestore, lightNodes, logger, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, s)

	_ = s.Ready()

	return s, overlay
}
