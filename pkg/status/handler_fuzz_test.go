// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/status"
	"github.com/ethersphere/bee/v2/pkg/status/internal/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzPeerSnapshot drives the real client protocol logic in
// Service.PeerSnapshot end-to-end over the streamtest recorder against a peer
// that returns hostile bytes. It exercises stream setup, the Get write, the
// length-delimited read+decode of the Snapshot response, and the final cast to
// *status.Snapshot — the full path a malicious responding peer can reach in the
// requesting node.
//
// The target asserts PeerSnapshot never panics and that a nil-error return is
// always paired with a non-nil snapshot.
func FuzzPeerSnapshot(f *testing.F) {
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

	address := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")

	f.Fuzz(func(t *testing.T, data []byte) {
		// A malicious server: consume the incoming Get request, then emit the
		// fuzzed raw bytes directly onto the stream as the "response".
		spec := p2p.ProtocolSpec{
			Name:    status.ProtocolName,
			Version: status.ProtocolVersion,
			StreamSpecs: []p2p.StreamSpec{{
				Name: status.StreamName,
				Handler: func(ctx context.Context, _ p2p.Peer, stream p2p.Stream) error {
					defer func() { _ = stream.FullClose() }()
					_, r := protobuf.NewWriterAndReader(stream)
					var g pb.Get
					if err := r.ReadMsgWithContext(ctx, &g); err != nil {
						return err
					}
					_, _ = stream.Write(data)
					return nil
				},
			}},
		}

		recorder := streamtest.New(streamtest.WithProtocols(spec))

		svc := status.NewService(log.Noop, recorder, new(topologyPeersIterNoopMock), "", nil, nil, nil)

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		ss, err := svc.PeerSnapshot(ctx, address)
		if err != nil {
			return
		}
		// PeerSnapshot returns a freshly allocated snapshot on success, so a
		// nil-error/nil-snapshot pair would indicate a real defect.
		if ss == nil {
			t.Fatal("PeerSnapshot returned nil error but nil snapshot")
		}
	})
}
