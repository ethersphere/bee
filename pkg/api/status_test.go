// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"net/http"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/status"
	"github.com/ethersphere/bee/v2/pkg/topology"
)

func TestGetStatus(t *testing.T) {
	t.Parallel()

	const url = "/status"

	t.Run("node", func(t *testing.T) {
		t.Parallel()

		mode := api.FullMode
		ssr := api.StatusSnapshotResponse{
			BeeMode:                 mode.String(),
			ReserveSize:             128,
			ReserveSizeWithinRadius: 64,
			PullsyncRate:            64,
			StorageRadius:           8,
			ConnectedPeers:          0,
			NeighborhoodSize:        0,
			BatchCommitment:         1,
			IsReachable:             true,
		}

		ssMock := &statusSnapshotMock{
			syncRate:                ssr.PullsyncRate,
			reserveSize:             int(ssr.ReserveSize),
			reserveSizeWithinRadius: ssr.ReserveSizeWithinRadius,
			storageRadius:           ssr.StorageRadius,
			commitment:              ssr.BatchCommitment,
		}

		statusSvc := status.NewService(
			log.Noop,
			nil,
			new(topologyPeersIterNoopMock),
			mode.String(),
			ssMock,
			ssMock,
		)

		statusSvc.SetSync(ssMock)

		client, _, _, _ := newTestServer(t, testServerOptions{
			BeeMode:    mode,
			NodeStatus: statusSvc,
		})

		jsonhttptest.Request(t, client, http.MethodGet, url, http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(ssr),
		)
	})

	t.Run("bad request", func(t *testing.T) {
		t.Parallel()

		client, _, _, _ := newTestServer(t, testServerOptions{
			BeeMode: api.DevMode,
			NodeStatus: status.NewService(
				log.Noop,
				nil,
				new(topologyPeersIterNoopMock),
				"",
				nil,
				nil,
			),
		})

		jsonhttptest.Request(t, client, http.MethodGet, url, http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: api.ErrUnsupportedDevNodeOperation.Error(),
				Code:    http.StatusBadRequest,
			}),
		)
	})
}

// topologyPeersIterNoopMock is noop topology.PeerIterator.
type topologyPeersIterNoopMock struct{}

func (m *topologyPeersIterNoopMock) EachConnectedPeer(_ topology.EachPeerFunc, _ topology.Select) error {
	return nil
}

func (m *topologyPeersIterNoopMock) EachConnectedPeerRev(_ topology.EachPeerFunc, _ topology.Select) error {
	return nil
}
func (m *topologyPeersIterNoopMock) IsReachable() bool {
	return true
}

// statusSnapshotMock satisfies the following interfaces:
//   - status.Reserve
//   - status.SyncReporter
//   - postage.CommitmentGetter
type statusSnapshotMock struct {
	syncRate                float64
	reserveSize             int
	reserveSizeWithinRadius uint64
	storageRadius           uint8
	commitment              uint64
}

func (m *statusSnapshotMock) SyncRate() float64           { return m.syncRate }
func (m *statusSnapshotMock) ReserveSize() int            { return m.reserveSize }
func (m *statusSnapshotMock) StorageRadius() uint8        { return m.storageRadius }
func (m *statusSnapshotMock) Commitment() (uint64, error) { return m.commitment, nil }
func (m *statusSnapshotMock) ReserveSizeWithinRadius() uint64 {
	return m.reserveSizeWithinRadius
}
