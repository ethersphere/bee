// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	topologymock "github.com/ethersphere/bee/v2/pkg/topology/mock"
)

func TestTopologyOK(t *testing.T) {
	t.Parallel()

	testServer, _, _, _ := newTestServer(t, testServerOptions{})

	var body []byte
	opts := jsonhttptest.WithPutResponseBody(&body)
	jsonhttptest.Request(t, testServer, http.MethodGet, "/topology", http.StatusOK, opts)

	if len(body) == 0 {
		t.Error("empty response")
	}
}

func TestTopology_SessionConnectionUnderlay(t *testing.T) {
	t.Parallel()

	peerAddr := swarm.MustParseHexAddress("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	underlayAddr := "/ip6/2001:db8::1/tcp/1634"

	expectedSnapshot := &topology.KadParams{
		Connected:  1,
		Population: 1,
		Bins: topology.KadBins{
			Bin0: topology.BinInfo{
				BinPopulation: 1,
				BinConnected:  1,
				ConnectedPeers: []*topology.PeerInfo{
					{
						Address: peerAddr,
						Metrics: &topology.MetricSnapshotView{
							SessionConnectionDirection: "inbound",
							SessionConnectionUnderlay:  underlayAddr,
							Reachability:               "public",
							Healthy:                    true,
						},
					},
				},
			},
		},
	}

	testServer, _, _, _ := newTestServer(t, testServerOptions{
		TopologyOpts: []topologymock.Option{
			topologymock.WithSnapshot(expectedSnapshot),
		},
	})

	var resp topology.KadParams
	opts := jsonhttptest.WithUnmarshalJSONResponse(&resp)
	jsonhttptest.Request(t, testServer, http.MethodGet, "/topology", http.StatusOK, opts)

	if resp.Connected != 1 {
		t.Fatalf("expected 1 connected peer, got %d", resp.Connected)
	}
	peers := resp.Bins.Bin0.ConnectedPeers
	if len(peers) != 1 {
		t.Fatalf("expected 1 connected peer in bin 0, got %d", len(peers))
	}
	if peers[0].Metrics == nil {
		t.Fatal("expected non-nil metrics")
	}
	if have, want := peers[0].Metrics.SessionConnectionDirection, "inbound"; have != want {
		t.Errorf("expected sessionConnectionDirection %q, got %q", want, have)
	}
	if have, want := peers[0].Metrics.SessionConnectionUnderlay, underlayAddr; have != want {
		t.Errorf("expected sessionConnectionUnderlay %q, got %q", want, have)
	}
}

func TestTopology_SessionConnectionUnderlay_OmitEmpty(t *testing.T) {
	t.Parallel()

	peerAddr := swarm.MustParseHexAddress("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	expectedSnapshot := &topology.KadParams{
		Connected:  1,
		Population: 1,
		Bins: topology.KadBins{
			Bin0: topology.BinInfo{
				BinPopulation: 1,
				BinConnected:  1,
				ConnectedPeers: []*topology.PeerInfo{
					{
						Address: peerAddr,
						Metrics: &topology.MetricSnapshotView{
							SessionConnectionDirection: "inbound",
							Reachability:               "public",
							Healthy:                    true,
						},
					},
				},
			},
		},
	}

	testServer, _, _, _ := newTestServer(t, testServerOptions{
		TopologyOpts: []topologymock.Option{
			topologymock.WithSnapshot(expectedSnapshot),
		},
	})

	var body []byte
	opts := jsonhttptest.WithPutResponseBody(&body)
	jsonhttptest.Request(t, testServer, http.MethodGet, "/topology", http.StatusOK, opts)

	if strings.Contains(string(body), "sessionConnectionUnderlay") {
		t.Errorf("expected sessionConnectionUnderlay to be omitted from JSON when empty, got: %s", string(body))
	}
}
