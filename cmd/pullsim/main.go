// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command pullsim runs an in-memory pull-sync network simulator with a browser
// UI. It spins up N synthetic nodes in one process, connected in memory, each
// running the real pkg/pullsync Syncer and pkg/puller Puller against an
// in-memory reserve, and streams chunk propagation to a web front-end.
//
// Goroutine budget is roughly N*(N-1)*Bins*2 live sync workers for a full
// mesh; prefer -topology k-nearest for larger N.
package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ethersphere/bee/v2/cmd/pullsim/internal/sim"
	"github.com/ethersphere/bee/v2/cmd/pullsim/internal/web"
	"github.com/ethersphere/bee/v2/pkg/log"
)

func main() {
	var (
		listen   = flag.String("listen", ":8080", "HTTP listen address")
		nodes    = flag.Int("nodes", 20, "number of nodes (10-50 recommended)")
		bins     = flag.Uint("bins", 8, "number of bins")
		topology = flag.String("topology", "full", "topology: full|ring|k-nearest|random")
		degree   = flag.Int("degree", 6, "peer degree (ring/k-nearest/random)")
		radius   = flag.Uint("radius", 0, "initial storage radius (< bins)")
		latency  = flag.Duration("latency", 5*time.Millisecond, "per-message latency")
		maxpage  = flag.Uint64("maxpage", 64, "pullsync max page size")
		clusters = flag.Int("clusters", 1, "address clusters (>1 for crisp neighborhoods)")
		seed     = flag.Int64("seed", 0, "RNG seed")
		verbose  = flag.Bool("v", false, "verbose protocol logging")
	)
	flag.Parse()

	logger := log.Noop
	if *verbose {
		logger = log.NewLogger(
			"pullsim",
			log.WithSink(os.Stdout),
			log.WithVerbosity(log.VerbosityDebug),
			log.WithTimestamp(),
		)
	}

	cfg := sim.Config{
		Nodes:    *nodes,
		Bins:     uint8(*bins),
		Topology: sim.Topology(*topology),
		Degree:   *degree,
		Radius:   uint8(*radius),
		Latency:  *latency,
		MaxPage:  *maxpage,
		Clusters: *clusters,
		Seed:     *seed,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	server, err := web.NewServer(ctx, cfg, logger)
	if err != nil {
		logger.Error(err, "failed to build network")
		os.Exit(1)
	}
	defer server.Close()

	httpSrv := &http.Server{
		Addr:              *listen,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("pullsim listening", "addr", *listen)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(err, "http server failed")
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
}
