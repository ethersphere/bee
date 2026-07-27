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
	"errors"
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
		settle   = flag.Duration("settle", 3*time.Second, "batch quiescence window before a batch counts as done propagating")
		verbose  = flag.Bool("v", false, "verbose protocol logging")

		bench       = flag.Bool("bench", false, "run a headless propagation sweep instead of the HTTP server")
		benchNodes  = flag.String("bench-nodes", "10,20,30,40,50", "sweep: comma-separated node counts")
		benchChunks = flag.String("bench-chunks", "1,10,100", "sweep: comma-separated batch sizes")
		benchReps   = flag.Int("bench-reps", 3, "sweep: repetitions per cell")
		benchWarmup = flag.Duration("bench-warmup", 5*time.Second, "sweep: settling time after start before injecting")
		benchSettle = flag.Duration("bench-settle", 3*time.Second, "sweep: override -settle for the sweep only")
		benchMinPO  = flag.Uint("bench-minpo", 0, "sweep: proximity order the injected chunks are mined to (set >= -radius)")
		benchTimeo  = flag.Duration("bench-timeout", 120*time.Second, "sweep: per-cell hard cap")
		benchOut    = flag.String("bench-out", "", "sweep: CSV output path (default stdout)")
	)
	flag.Parse()

	// -settle governs both modes; -bench-settle only overrides it when the
	// operator actually passed it, so the two modes cannot silently diverge.
	settleAfter := *settle
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "bench-settle" && *bench {
			settleAfter = *benchSettle
		}
	})

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
		Nodes:       *nodes,
		Bins:        uint8(*bins),
		Topology:    sim.Topology(*topology),
		Degree:      *degree,
		Radius:      uint8(*radius),
		Latency:     *latency,
		MaxPage:     *maxpage,
		Clusters:    *clusters,
		Seed:        *seed,
		SettleAfter: settleAfter,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if *bench {
		// The sweep prints CSV to stdout, so its logger must always go to
		// stderr regardless of -v — reusing the stdout logger here would
		// interleave debug lines into the CSV and corrupt it. Only the
		// verbosity level follows -v.
		v := log.VerbosityInfo
		if *verbose {
			v = log.VerbosityDebug
		}
		benchLogger := log.NewLogger("pullsim", log.WithSink(os.Stderr), log.WithVerbosity(v), log.WithTimestamp())
		nodesGrid, err := parseGrid(*benchNodes)
		if err != nil {
			benchLogger.Error(err, "invalid -bench-nodes")
			os.Exit(1)
		}
		for _, n := range nodesGrid {
			if n < 2 {
				benchLogger.Error(nil, "invalid -bench-nodes: node count must be >= 2", "value", n)
				os.Exit(1)
			}
		}
		chunksGrid, err := parseGrid(*benchChunks)
		if err != nil {
			benchLogger.Error(err, "invalid -bench-chunks")
			os.Exit(1)
		}
		if *benchReps < 1 {
			benchLogger.Error(nil, "-bench-reps must be >= 1")
			os.Exit(1)
		}
		if *benchMinPO > 255 || uint8(*benchMinPO) >= uint8(*bins) {
			benchLogger.Error(nil, "-bench-minpo must be < -bins", "bench-minpo", *benchMinPO, "bins", *bins)
			os.Exit(1)
		}
		if uint8(*benchMinPO) < uint8(*radius) {
			benchLogger.Warning("-bench-minpo is below -radius: most offered chunks fall outside the receiving node's "+
				"neighborhood and will not be stored, so cells will report no-replicas",
				"bench-minpo", *benchMinPO, "radius", *radius)
		}
		opts := benchOptions{
			Base:    cfg,
			Nodes:   nodesGrid,
			Chunks:  chunksGrid,
			Reps:    *benchReps,
			Warmup:  *benchWarmup,
			Settle:  settleAfter,
			MinPO:   uint8(*benchMinPO),
			Timeout: *benchTimeo,
			Out:     *benchOut,
		}
		if err := runBench(ctx, opts, benchLogger); err != nil && !errors.Is(err, context.Canceled) {
			benchLogger.Error(err, "sweep failed")
			os.Exit(1)
		}
		return
	}

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
