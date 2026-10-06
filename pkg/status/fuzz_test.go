// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status_test

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/status"
	"github.com/ethersphere/bee/v2/pkg/status/internal/pb"
)

func statusFrame(tb testing.TB, msg protobuf.Message) []byte {
	tb.Helper()
	var buf bytes.Buffer
	if err := protobuf.NewWriter(&buf).WriteMsg(msg); err != nil {
		tb.Fatal(err)
	}
	return buf.Bytes()
}

// FuzzSnapshotRead fuzzes the client-side decode path in Service.PeerSnapshot:
// the production protobuf reader decoding a peer-supplied pb.Snapshot, followed
// by the same conversion to the exported status.Snapshot type that a caller
// consumes. It asserts the combined path never panics on hostile input.
func FuzzSnapshotRead(f *testing.F) {
	f.Add(statusFrame(f, &pb.Snapshot{
		BeeMode:         "full",
		ReserveSize:     128,
		StorageRadius:   8,
		BatchCommitment: 1024,
		LastSyncedBlock: 6092500,
		Metrics:         map[string]string{"test_metric": "42"},
	}))
	f.Add(statusFrame(f, &pb.Snapshot{}))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := protobuf.NewReader(bytes.NewReader(data))
		var m pb.Snapshot
		if err := r.ReadMsg(&m); err != nil {
			return
		}
		// Mirror how PeerSnapshot returns the result and how a caller reads it:
		// cast to the exported type and touch every field, iterating Metrics.
		s := (*status.Snapshot)(&m)
		_ = s.BeeMode
		_ = s.ReserveSize
		_ = s.ReserveSizeWithinRadius
		_ = s.PullsyncRate
		_ = s.StorageRadius
		_ = s.ConnectedPeers
		_ = s.NeighborhoodSize
		_ = s.BatchCommitment
		_ = s.IsReachable
		_ = s.LastSyncedBlock
		_ = s.CommittedDepth
		for k, v := range s.Metrics {
			_, _ = k, v
		}
	})
}

// FuzzGetRead fuzzes the server-side request-parse path in Service.handler:
// the production protobuf reader decoding a peer-supplied pb.Get. Get is an
// empty message, so this guards the request-parse path against malformed or
// oversized frames. No-panic only.
func FuzzGetRead(f *testing.F) {
	f.Add(statusFrame(f, &pb.Get{}))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := protobuf.NewReader(bytes.NewReader(data))
		var g pb.Get
		if err := r.ReadMsg(&g); err != nil {
			return
		}
		_ = g
	})
}
