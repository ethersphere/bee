// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/cmd/pullsim/internal/sim"
	"github.com/ethersphere/bee/v2/cmd/pullsim/internal/web"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/gorilla/websocket"
)

func httpGet(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func httpPost(t *testing.T, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	// A 2-node network keeps exactly one client per server, so the pullsync
	// server never coalesces two peers on one singleflight key. That avoids the
	// upstream resenje.org/singleflight data race (see sim/network_test.go) and
	// lets these HTTP/websocket plumbing tests run cleanly under -race.
	return newTestServerN(t, 2)
}

// newTestServerN is newTestServer with an explicit node count, for the churn
// tests: Churn refuses to leave fewer than 2 survivors, so a successful
// departure needs at least 3 nodes.
func newTestServerN(t *testing.T, nodes int) *httptest.Server {
	t.Helper()
	srv, err := web.NewServer(context.Background(), sim.Config{
		Nodes: nodes, Bins: 4, Topology: sim.TopologyFull, Radius: 0, Seed: 7,
	}, log.Noop)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() {
		ts.Close()
		srv.Close()
	})
	return ts
}

func TestServer_NetworkAndInject(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	resp := httpGet(t, ts.URL+"/api/network")
	defer resp.Body.Close()
	var net struct {
		Config   map[string]any `json:"config"`
		Snapshot struct {
			Nodes []any `json:"nodes"`
		} `json:"snapshot"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&net); err != nil {
		t.Fatal(err)
	}
	if len(net.Snapshot.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(net.Snapshot.Nodes))
	}

	ir := httpPost(t, ts.URL+"/api/inject", `{"node":0,"count":1,"minPo":0}`)
	defer ir.Body.Close()
	if ir.StatusCode != http.StatusOK {
		t.Fatalf("inject failed: %d", ir.StatusCode)
	}
	var inj struct {
		Addrs []string `json:"addrs"`
	}
	if err := json.NewDecoder(ir.Body).Decode(&inj); err != nil {
		t.Fatal(err)
	}
	if len(inj.Addrs) != 1 {
		t.Fatalf("expected 1 injected addr, got %d", len(inj.Addrs))
	}
}

// TestServer_RebuildKeepsSyncing guards against starting the rebuilt network
// with the HTTP request context: that context is cancelled the moment the
// rebuild response is written, which silently kills every puller.
func TestServer_RebuildKeepsSyncing(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	rb := httpPost(t, ts.URL+"/api/network",
		`{"nodes":2,"bins":4,"topology":"full","radius":0,"seed":9}`)
	rb.Body.Close()
	if rb.StatusCode != http.StatusOK {
		t.Fatalf("rebuild failed: %d", rb.StatusCode)
	}

	ir := httpPost(t, ts.URL+"/api/inject", `{"node":0,"count":4,"minPo":0}`)
	ir.Body.Close()
	if ir.StatusCode != http.StatusOK {
		t.Fatalf("inject failed: %d", ir.StatusCode)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		resp := httpGet(t, ts.URL+"/api/network")
		var net struct {
			Snapshot struct {
				Nodes []struct {
					ReserveSize int `json:"reserveSize"`
				} `json:"nodes"`
			} `json:"snapshot"`
		}
		err := json.NewDecoder(resp.Body).Decode(&net)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if len(net.Snapshot.Nodes) == 2 && net.Snapshot.Nodes[1].ReserveSize > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("node 1 never synced any chunk after rebuild")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestServer_WebsocketHello(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, frame, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(frame, &m); err != nil {
		t.Fatal(err)
	}
	if m["t"] != "hello" {
		t.Fatalf("expected hello frame, got %v", m["t"])
	}
	if _, ok := m["snap"]; !ok {
		t.Fatal("hello frame missing snapshot")
	}
}

// churnResult mirrors sim.ChurnResult's json tags, which are the shape the UI
// consumes; decoding into it here pins that contract.
type churnResult struct {
	Departed     []int `json:"departed"`
	Survivors    int   `json:"survivors"`
	Lost         int   `json:"lost"`
	EdgesAdded   int   `json:"edgesAdded"`
	EdgesRemoved int   `json:"edgesRemoved"`
}

func postChurn(t *testing.T, ts *httptest.Server, body string) (*http.Response, churnResult) {
	t.Helper()
	resp := httpPost(t, ts.URL+"/api/churn", body)
	defer resp.Body.Close()
	var res churnResult
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			t.Fatal(err)
		}
	}
	return resp, res
}

func TestServer_ChurnCount(t *testing.T) {
	t.Parallel()
	ts := newTestServerN(t, 3)

	resp, res := postChurn(t, ts, `{"count":1}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("churn failed: %d", resp.StatusCode)
	}
	if len(res.Departed) != 1 {
		t.Fatalf("expected 1 departure, got %v", res.Departed)
	}
	if res.Survivors != 2 {
		t.Fatalf("expected 2 survivors, got %d", res.Survivors)
	}
}

func TestServer_ChurnNodes(t *testing.T) {
	t.Parallel()
	ts := newTestServerN(t, 3)

	resp, res := postChurn(t, ts, `{"nodes":[1]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("churn failed: %d", resp.StatusCode)
	}
	if len(res.Departed) != 1 || res.Departed[0] != 1 {
		t.Fatalf("expected node 1 to depart, got %v", res.Departed)
	}
	if res.Survivors != 2 {
		t.Fatalf("expected 2 survivors, got %d", res.Survivors)
	}
}

func TestServer_ChurnRejectsBadBody(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		body string
	}{
		{"both", `{"count":1,"nodes":[1]}`},
		{"neither", `{}`},
		// Every sim-side rejection is a bad request, not a server fault: on a
		// 2-node network departing one node would leave a single survivor.
		{"too few survivors", `{"count":1}`},
		{"out of range", `{"nodes":[9]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ts := newTestServer(t)
			resp, _ := postChurn(t, ts, tc.body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", resp.StatusCode)
			}
		})
	}
}

func TestServer_ServesStatic(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	resp := httpGet(t, ts.URL+"/")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("index not served: %d", resp.StatusCode)
	}
}
