// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync_test

import (
	"context"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/topology/mock"
)

// TestHandlerChunkValidity pushes valid and invalid CACs and SOCs to a peer
// and checks that only the valid ones are accepted and stored.
func TestHandlerChunkValidity(t *testing.T) {
	t.Parallel()

	for _, tc := range testingc.ChunkValidityCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")
			closestPeer := swarm.MustParseHexAddress("8000000000000000000000000000000000000000000000000000000000000000")

			psPeer, peerStorer, _ := createPushSyncNode(t, closestPeer, defaultPrices, nil, nil, defaultSigner(tc.Chunk), mock.WithClosestPeerErr(topology.ErrWantSelf))
			recorder := streamtest.New(streamtest.WithProtocols(psPeer.Protocol()), streamtest.WithBaseAddr(pivotNode))
			psPivot, _, _ := createPushSyncNode(t, pivotNode, defaultPrices, recorder, nil, defaultSigner(tc.Chunk), mock.WithClosestPeer(closestPeer))

			_, err := psPivot.PushChunkToClosest(context.Background(), tc.Chunk)
			stored := peerStorer.hasChunk(t, tc.Chunk.Address())

			if tc.Valid {
				if err != nil {
					t.Fatalf("push: unexpected error: %v", err)
				}
				if !stored {
					t.Fatal("valid chunk not stored")
				}
				return
			}
			if err == nil {
				t.Error("push: expected error, got nil")
			}
			if stored {
				t.Error("invalid chunk stored")
			}
		})
	}
}
