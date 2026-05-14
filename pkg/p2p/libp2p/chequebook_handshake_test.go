// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p_test

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestService_DisconnectedEvictsChequebookEntry drives the unexported
// disconnect hook through the Disconnected export and asserts that a
// peer→chequebook mapping seeded into a real chequebook.Registry is
// removed end-to-end. The registry-level Remove is covered separately in
// pkg/settlement/swap/chequebook/registry_test.go; this test proves the
// wiring through Service.disconnected reaches it.
func TestService_DisconnectedEvictsChequebookEntry(t *testing.T) {
	t.Parallel()

	registry := chequebook.NewRegistry()
	overlay := swarm.RandAddress(t)
	cb := common.HexToAddress("0x1111111111111111111111111111111111111111")

	if err := registry.Put(overlay, cb, time.Now().Unix(), bzz.TimestampSourceHandshake, nil); err != nil {
		t.Fatalf("seed registry: %v", err)
	}
	if _, ok := registry.Peer(cb); !ok {
		t.Fatal("pre-condition: registry must contain seeded entry")
	}

	svc := libp2p.NewDisconnectTestService(log.Noop, registry)

	svc.Disconnected(overlay)

	if _, ok := registry.Peer(cb); ok {
		t.Fatal("registry entry must be evicted after Disconnected")
	}
}
