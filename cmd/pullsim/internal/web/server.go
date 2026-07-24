// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package web serves the pullsim control API and the browser UI. It knows
// nothing about the pull-sync protocol; it drives the sim.Network through its
// public API and streams event.Bus frames to connected browsers.
package web

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/cmd/pullsim/internal/event"
	"github.com/ethersphere/bee/v2/cmd/pullsim/internal/sim"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/gorilla/mux"
)

//go:embed static
var staticFS embed.FS

// Server owns the event bus and the current network, rebuilding the latter on
// demand behind a stable bus so websocket clients survive rebuilds.
type Server struct {
	logger log.Logger

	// baseCtx has the process lifetime; networks are started with it so a
	// rebuild triggered by a short-lived HTTP request is not cancelled with
	// that request.
	baseCtx context.Context

	mu  sync.Mutex
	net *sim.Network
	bus *event.Bus
}

// NewServer builds the initial network and starts the bus and pullers.
func NewServer(ctx context.Context, cfg sim.Config, logger log.Logger) (*Server, error) {
	s := &Server{logger: logger, baseCtx: ctx}
	s.bus = event.NewBus(event.ProviderFunc(s.snapshot))
	s.bus.Start()

	if err := s.build(cfg); err != nil {
		s.bus.Close()
		return nil, err
	}
	return s, nil
}

// build closes any existing network and starts a fresh one. Caller must not
// hold s.mu.
func (s *Server) build(cfg sim.Config) error {
	n, err := sim.BuildNetwork(cfg, s.logger)
	if err != nil {
		return err
	}
	n.SetBus(s.bus)

	s.mu.Lock()
	old := s.net
	s.net = n
	s.mu.Unlock()

	if old != nil {
		old.Close()
	}
	n.Start(s.baseCtx)

	c := n.Config()
	s.bus.Publish(event.Config{
		Nodes: c.Nodes, Bins: c.Bins, Topology: string(c.Topology), Degree: c.Degree,
		Radius: c.Radius, MaxPage: c.MaxPage, LatencyMs: c.Latency.Milliseconds(),
		Clusters: c.Clusters, Seed: c.Seed,
	})
	return nil
}

func (s *Server) current() *sim.Network {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.net
}

func (s *Server) snapshot() event.Snapshot {
	n := s.current()
	if n == nil {
		return event.Snapshot{}
	}
	return n.Snapshot()
}

// Handler builds the HTTP router.
func (s *Server) Handler() http.Handler {
	r := mux.NewRouter()
	r.HandleFunc("/ws", s.handleWS)
	r.HandleFunc("/api/network", s.handleNetwork).Methods(http.MethodGet, http.MethodPost)
	r.HandleFunc("/api/radius", s.handleRadius).Methods(http.MethodPost)
	r.HandleFunc("/api/inject", s.handleInject).Methods(http.MethodPost)
	r.HandleFunc("/api/inject/stop", s.handleInjectStop).Methods(http.MethodPost)

	sub, _ := fs.Sub(staticFS, "static")
	r.PathPrefix("/").Handler(http.FileServer(http.FS(sub)))
	return r
}

// Close shuts down the network and the bus.
func (s *Server) Close() {
	if n := s.current(); n != nil {
		n.Close()
	}
	s.bus.Close()
}

// --- REST handlers ---

type configJSON struct {
	Nodes     int    `json:"nodes"`
	Bins      uint8  `json:"bins"`
	Topology  string `json:"topology"`
	Degree    int    `json:"degree"`
	Radius    uint8  `json:"radius"`
	LatencyMs int64  `json:"latencyMs"`
	MaxPage   uint64 `json:"maxPage"`
	Clusters  int    `json:"clusters"`
	Seed      int64  `json:"seed"`
}

func (c configJSON) toSim() sim.Config {
	return sim.Config{
		Nodes:    c.Nodes,
		Bins:     c.Bins,
		Topology: sim.Topology(c.Topology),
		Degree:   c.Degree,
		Radius:   c.Radius,
		Latency:  time.Duration(c.LatencyMs) * time.Millisecond,
		MaxPage:  c.MaxPage,
		Clusters: c.Clusters,
		Seed:     c.Seed,
	}
}

func fromSim(c sim.Config) configJSON {
	return configJSON{
		Nodes: c.Nodes, Bins: c.Bins, Topology: string(c.Topology), Degree: c.Degree,
		Radius: c.Radius, LatencyMs: c.Latency.Milliseconds(), MaxPage: c.MaxPage,
		Clusters: c.Clusters, Seed: c.Seed,
	}
}

func (s *Server) handleNetwork(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var cfg configJSON
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.build(cfg.toSim()); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	n := s.current()
	writeJSON(w, http.StatusOK, map[string]any{
		"config":   fromSim(n.Config()),
		"snapshot": n.Snapshot(),
	})
}

func (s *Server) handleRadius(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Radius uint8 `json:"radius"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	n := s.current()
	if n == nil {
		writeError(w, http.StatusServiceUnavailable, "no network")
		return
	}
	n.SetRadius(body.Radius)
	writeJSON(w, http.StatusOK, map[string]any{"radius": n.Radius()})
}

func (s *Server) handleInject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Node  int     `json:"node"`
		Count int     `json:"count"`
		Rate  float64 `json:"rate"`
		MinPO uint8   `json:"minPo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	n := s.current()
	if n == nil {
		writeError(w, http.StatusServiceUnavailable, "no network")
		return
	}
	if body.Count == 0 {
		body.Count = 1
	}
	addrs, err := n.Inject(body.Node, body.Count, body.Rate, body.MinPO)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	strs := make([]string, len(addrs))
	for i, a := range addrs {
		strs[i] = a.String()
	}
	writeJSON(w, http.StatusOK, map[string]any{"addrs": strs})
}

func (s *Server) handleInjectStop(w http.ResponseWriter, r *http.Request) {
	if n := s.current(); n != nil {
		n.StopInject()
	}
	writeJSON(w, http.StatusOK, map[string]any{"stopped": true})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}
