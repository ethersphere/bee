// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/status"
	"github.com/ethersphere/bee/pkg/status/internal/pb"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/google/go-cmp/cmp"
)

func TestStatus(t *testing.T) {
	t.Parallel()

	want := &pb.Snapshot{
		BeeMode:       api.FullMode.String(),
		ReserveSize:   128,
		PullsyncRate:  64,
		StorageRadius: 8,
	}

	sssMock := &statusSnapshotMock{want}

	peersIterMock := new(topologyPeersIterNoopMock)

	peer1 := status.NewService(
		log.Noop,
		nil,
		peersIterMock,
		want.BeeMode,
		sssMock,
		sssMock,
		sssMock,
	)

	recorder := streamtest.New(streamtest.WithProtocols(peer1.Protocol()))

	peer2 := status.NewService(log.Noop, recorder, peersIterMock, "", nil, nil, nil)

	address := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")

	if _, err := peer2.PeerSnapshot(context.Background(), address); err != nil {
		t.Fatalf("send msg get: unexpected error: %v", err)
	}

	records, err := recorder.Records(address, status.ProtocolName, status.ProtocolVersion, status.StreamName)
	if err != nil {
		t.Fatal(err)
	}
	if have, want := len(records), 1; want != have {
		t.Fatalf("have %v records, want %v", have, want)
	}

	messages, err := protobuf.ReadMessages(
		bytes.NewReader(records[0].In()),
		func() protobuf.Message { return new(pb.Get) },
	)
	if err != nil {
		t.Fatalf("read messages: unexpected error: %v", err)
	}
	if have, want := len(messages), 1; want != have {
		t.Fatalf("have %v messages, want %v", have, want)
	}

	messages, err = protobuf.ReadMessages(
		bytes.NewReader(records[0].Out()),
		func() protobuf.Message { return new(pb.Snapshot) },
	)
	if err != nil {
		t.Fatalf("read messages: unexpected error: %v", err)
	}
	have := messages[0].(*pb.Snapshot)

	if diff := cmp.Diff(want, have); diff != "" {
		t.Fatalf("unexpected snapshot (-want +have):\n%s", diff)
	}
}

// topologyPeersIterNoopMock is noop topology.PeerIterator.
type topologyPeersIterNoopMock struct{}

func (m *topologyPeersIterNoopMock) EachConnectedPeer(_ topology.EachPeerFunc, _ topology.Filter) error {
	return nil
}

func (m *topologyPeersIterNoopMock) EachConnectedPeerRev(_ topology.EachPeerFunc, _ topology.Filter) error {
	return nil
}

// statusSnapshotMock satisfies the following interfaces:
//   - depthmonitor.ReserveReporter
//   - depthmonitor.SyncReporter
//   - postage.RadiusReporter
type statusSnapshotMock struct {
	*pb.Snapshot
}

func (m *statusSnapshotMock) SyncRate() float64    { return m.Snapshot.PullsyncRate }
func (m *statusSnapshotMock) ReserveSize() uint64  { return m.Snapshot.ReserveSize }
func (m *statusSnapshotMock) StorageRadius() uint8 { return uint8(m.Snapshot.StorageRadius) }
