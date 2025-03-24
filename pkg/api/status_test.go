// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/status"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/prometheus/client_golang/prometheus"
)

func TestGetStatus(t *testing.T) {
	t.Parallel()

	const url = "/status"

	t.Run("node", func(t *testing.T) {
		t.Parallel()

		mode := api.FullMode
		ssr := api.StatusSnapshotResponse{
			Proximity:               256,
			BeeMode:                 mode.String(),
			ReserveSize:             128,
			ReserveSizeWithinRadius: 64,
			PullsyncRate:            64,
			StorageRadius:           8,
			ConnectedPeers:          0,
			NeighborhoodSize:        1,
			BatchCommitment:         1,
			IsReachable:             true,
			LastSyncedBlock:         6092500,
			CommittedDepth:          1,
			Metrics: `# HELP test_response_duration_seconds Histogram of API response durations.
# TYPE test_response_duration_seconds histogram
test_response_duration_seconds_bucket{test="label",le="0.01"} 1
test_response_duration_seconds_bucket{test="label",le="0.1"} 1
test_response_duration_seconds_bucket{test="label",le="0.25"} 2
test_response_duration_seconds_bucket{test="label",le="0.5"} 2
test_response_duration_seconds_bucket{test="label",le="1"} 3
test_response_duration_seconds_bucket{test="label",le="2.5"} 4
test_response_duration_seconds_bucket{test="label",le="5"} 4
test_response_duration_seconds_bucket{test="label",le="10"} 6
test_response_duration_seconds_bucket{test="label",le="+Inf"} 7
test_response_duration_seconds_sum{test="label"} 78.15
test_response_duration_seconds_count{test="label"} 7
`,
		}

		ssMock := &statusSnapshotMock{
			syncRate:                ssr.PullsyncRate,
			reserveSize:             int(ssr.ReserveSize),
			reserveSizeWithinRadius: ssr.ReserveSizeWithinRadius,
			storageRadius:           ssr.StorageRadius,
			commitment:              ssr.BatchCommitment,
			chainState:              &postage.ChainState{Block: ssr.LastSyncedBlock},
			committedDepth:          ssr.CommittedDepth,
		}

		metricsRegistry := prometheus.NewRegistry()

		h := prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "test",
			Name:      "response_duration_seconds",
			Help:      "Histogram of API response durations.",
			Buckets:   []float64{0.01, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			ConstLabels: prometheus.Labels{
				"test": "label",
			},
		})

		metricsRegistry.MustRegister(h)

		points := []float64{0.25, 5.2, 1.5, 1, 5.2, 0, 65}
		var sum float64
		for _, p := range points {
			h.Observe(p)
			sum += p
		}

		statusSvc := status.NewService(
			log.Noop,
			nil,
			new(topologyPeersIterNoopMock),
			mode.String(),
			ssMock,
			ssMock,
			metricsRegistry,
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
	chainState              *postage.ChainState
	neighborhoods           []*storer.NeighborhoodStat
	committedDepth          uint8
}

func (m *statusSnapshotMock) SyncRate() float64                  { return m.syncRate }
func (m *statusSnapshotMock) ReserveSize() int                   { return m.reserveSize }
func (m *statusSnapshotMock) StorageRadius() uint8               { return m.storageRadius }
func (m *statusSnapshotMock) Commitment() (uint64, error)        { return m.commitment, nil }
func (m *statusSnapshotMock) GetChainState() *postage.ChainState { return m.chainState }
func (m *statusSnapshotMock) ReserveSizeWithinRadius() uint64 {
	return m.reserveSizeWithinRadius
}
func (m *statusSnapshotMock) NeighborhoodsStat(ctx context.Context) ([]*storer.NeighborhoodStat, error) {
	return m.neighborhoods, nil
}
func (m *statusSnapshotMock) CommittedDepth() uint8 { return m.committedDepth }
