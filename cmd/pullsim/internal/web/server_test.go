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
	srv, err := web.NewServer(context.Background(), sim.Config{
		Nodes: 2, Bins: 4, Topology: sim.TopologyFull, Radius: 0, Seed: 7,
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

func TestServer_ServesStatic(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	resp := httpGet(t, ts.URL+"/")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("index not served: %d", resp.StatusCode)
	}
}
