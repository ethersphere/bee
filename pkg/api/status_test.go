// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"math/big"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/status"
	"github.com/ethersphere/bee/pkg/topology"
)

func TestGetStatus(t *testing.T) {
	t.Parallel()

	const url = "/status"

	t.Run("node", func(t *testing.T) {
		t.Parallel()

		mode := api.FullMode
		ssr := api.StatusSnapshotResponse{
			BeeMode:          mode.String(),
			ReserveSize:      128,
			PullsyncRate:     64,
			StorageRadius:    8,
			ConnectedPeers:   0,
			NeighborhoodSize: 0,
		}

		ssMock := &statusSnapshotMock{
			syncRate:      ssr.PullsyncRate,
			reserveSize:   ssr.ReserveSize,
			storageRadius: ssr.StorageRadius,
			chainstate:    &postage.ChainState{TotalAmount: big.NewInt(1)},
		}

		client, _, _, _ := newTestServer(t, testServerOptions{
			BeeMode:  mode,
			DebugAPI: true,
			NodeStatus: status.NewService(
				log.Noop,
				nil,
				new(topologyPeersIterNoopMock),
				mode.String(),
				ssMock,
				ssMock,
				ssMock,
				ssMock,
			),
		})

		jsonhttptest.Request(t, client, http.MethodGet, url, http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(ssr),
		)
	})

	t.Run("bad request", func(t *testing.T) {
		t.Parallel()

		client, _, _, _ := newTestServer(t, testServerOptions{
			BeeMode:  api.DevMode,
			DebugAPI: true,
			NodeStatus: status.NewService(
				log.Noop,
				nil,
				new(topologyPeersIterNoopMock),
				"",
				nil,
				nil,
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
	syncRate      float64
	reserveSize   uint64
	storageRadius uint8
	chainstate    *postage.ChainState
}

func (m *statusSnapshotMock) SyncRate() float64                  { return m.syncRate }
func (m *statusSnapshotMock) ReserveSize() uint64                { return m.reserveSize }
func (m *statusSnapshotMock) StorageRadius() uint8               { return m.storageRadius }
func (m *statusSnapshotMock) GetChainState() *postage.ChainState { return m.chainstate }
