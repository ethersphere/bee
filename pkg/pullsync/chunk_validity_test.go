// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullsync_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	mock "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestIncoming_ChunkValidity syncs valid and invalid CACs and SOCs from a
// peer and checks that only the valid ones are stored in the reserve.
func TestIncoming_ChunkValidity(t *testing.T) {
	for _, tc := range testingc.ChunkValidityCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				stampHash, err := tc.Chunk.Stamp().Hash()
				if err != nil {
					t.Fatal(err)
				}
				tResults := []*storer.BinC{{
					Address:   tc.Chunk.Address(),
					BatchID:   tc.Chunk.Stamp().BatchID(),
					BinID:     1,
					StampHash: stampHash,
				}}

				var (
					ps, _              = newPullSync(t, nil, 5, mock.WithSubscribeResp(tResults, nil), mock.WithChunks(tc.Chunk))
					recorder           = streamtest.New(streamtest.WithProtocols(ps.Protocol()))
					psClient, clientDb = newPullSync(t, recorder, 0)
				)

				_, _, err = psClient.Sync(context.Background(), swarm.ZeroAddress, 0, 0)

				if tc.Valid {
					if err != nil {
						t.Fatalf("sync: unexpected error: %v", err)
					}
					haveChunks(t, clientDb, tc.Chunk)
					return
				}
				if !errors.Is(err, swarm.ErrInvalidChunk) {
					t.Errorf("got error %v, want %v", err, swarm.ErrInvalidChunk)
				}
				if p := clientDb.PutCalls(); p != 0 {
					t.Errorf("invalid chunk stored in reserve: got %d puts, want 0", p)
				}
			})
		})
	}
}
