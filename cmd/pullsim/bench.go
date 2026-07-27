// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ethersphere/bee/v2/cmd/pullsim/internal/sim"
	"github.com/ethersphere/bee/v2/pkg/log"
)

// benchOptions parameterizes a sweep. The base config supplies the fixed
// backdrop (bins, topology, degree, radius, latency, maxpage, clusters, seed);
// Nodes is overridden per cell.
type benchOptions struct {
	Base   sim.Config
	Nodes  []int
	Chunks []int
	Reps   int
	Warmup time.Duration
	Settle time.Duration
	// MinPO is the proximity order the injected chunks are mined to. With a
	// non-zero Base.Radius it must be at least that radius, or a receiving
	// node only stores an offered chunk with probability 2^-radius and most
	// cells measure nothing.
	MinPO   uint8
	Timeout time.Duration
	Out     string
}

// benchRow is one measured cell.
type benchRow struct {
	Nodes     int
	Chunks    int
	Rep       int
	Topology  string
	Degree    int
	Radius    uint8
	Bins      uint8
	LatencyMs int64
	MaxPage   uint64
	Clusters  int
	Seed      int64
	SettleMs  int64
	WarmupMs  int64

	SpanMs   int64
	InjectMs int64
	TailMs   int64

	Replicas     int
	NodesReached int
	LateReplicas int

	PerDeliveryP50Ms int64
	PerDeliveryP95Ms int64
	PerDeliveryMaxMs int64

	// Status is the verdict for the cell:
	//   ok           settled, replicated, nothing arrived after the window
	//   truncated    settled, but replicas kept arriving afterwards
	//   no-replicas  nothing was ever replicated — the span is meaningless
	//   not-settled  the wait returned without the batch settling
	//   timeout      the per-cell cap expired
	//   error        the wait failed for any other reason
	// A non-ok cell still emits its observed counts rather than vanishing, so
	// a stall shows up as data.
	Status string
}

func benchHeader() []string {
	return []string{
		"nodes", "chunks", "rep",
		"topology", "degree", "radius", "bins", "latencyMs", "maxPage", "clusters", "seed",
		"settleMs", "warmupMs",
		"spanMs", "injectMs", "tailMs",
		"replicas", "nodesReached", "lateReplicas",
		"perDeliveryP50Ms", "perDeliveryP95Ms", "perDeliveryMaxMs",
		"status",
	}
}

func (r benchRow) csv() []string {
	// A batch that did not cleanly settle has no trustworthy timing: the
	// tracker never computed its percentiles, and its span ends at whatever
	// put happened to be last (for a batch with no replicas at all, the
	// origin's own inject microseconds after t0). Left as "0" those columns
	// read as the *fastest* cell in the sweep. Emit empty fields instead, so a
	// downstream analysis that sorts on timing rather than status cannot
	// mistake a non-measurement for a record. Counts stay real throughout —
	// they are what says *why* the cell is untrustworthy.
	span := strconv.FormatInt(r.SpanMs, 10)
	tail := strconv.FormatInt(r.TailMs, 10)
	p50 := strconv.FormatInt(r.PerDeliveryP50Ms, 10)
	p95 := strconv.FormatInt(r.PerDeliveryP95Ms, 10)
	max := strconv.FormatInt(r.PerDeliveryMaxMs, 10)
	if r.Status != "ok" {
		span, tail, p50, p95, max = "", "", "", "", ""
	}
	return []string{
		strconv.Itoa(r.Nodes), strconv.Itoa(r.Chunks), strconv.Itoa(r.Rep),
		r.Topology, strconv.Itoa(r.Degree), strconv.Itoa(int(r.Radius)),
		strconv.Itoa(int(r.Bins)), strconv.FormatInt(r.LatencyMs, 10),
		strconv.FormatUint(r.MaxPage, 10), strconv.Itoa(r.Clusters),
		strconv.FormatInt(r.Seed, 10),
		strconv.FormatInt(r.SettleMs, 10), strconv.FormatInt(r.WarmupMs, 10),
		span, strconv.FormatInt(r.InjectMs, 10), tail,
		strconv.Itoa(r.Replicas), strconv.Itoa(r.NodesReached), strconv.Itoa(r.LateReplicas),
		p50, p95, max,
		r.Status,
	}
}

// parseGrid parses a comma-separated list of positive integers.
func parseGrid(s string) ([]int, error) {
	fields := strings.Split(s, ",")
	out := make([]int, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			return nil, fmt.Errorf("empty value in grid %q", s)
		}
		v, err := strconv.Atoi(f)
		if err != nil {
			return nil, fmt.Errorf("bad value %q in grid: %w", f, err)
		}
		if v < 1 {
			return nil, fmt.Errorf("value %d in grid must be >= 1", v)
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, errors.New("grid is empty")
	}
	return out, nil
}

// cellSeed derives a per-cell RNG seed so repetitions of the same cell differ
// while the whole sweep stays reproducible from the base seed.
func cellSeed(base int64, nodes, chunks, rep int) int64 {
	h := base
	h = h*1099511628211 ^ int64(nodes)
	h = h*1099511628211 ^ int64(chunks)
	h = h*1099511628211 ^ int64(rep)
	return h
}

// runBench executes the sweep and writes CSV to opts.Out (stdout if empty).
func runBench(ctx context.Context, opts benchOptions, logger log.Logger) error {
	var w io.Writer = os.Stdout
	if opts.Out != "" {
		f, err := os.Create(opts.Out)
		if err != nil {
			return fmt.Errorf("create %s: %w", opts.Out, err)
		}
		defer func() { _ = f.Close() }()
		w = f
	}

	// The default backdrop is a full mesh at radius 0, where every node is a
	// direct peer of the origin and every node stores everything: propagation
	// is one hop by construction, so spanMs sits at pullsync's ~1s pageTimeout
	// no matter how many nodes there are. That is the opposite of a
	// size-scaling measurement, and it is quiet enough to be mistaken for a
	// result. Warn (on stderr, so the CSV on stdout stays clean) and proceed.
	if opts.Base.Topology == sim.TopologyFull && opts.Base.Radius == 0 {
		logger.Warning(
			"single-hop configuration: -topology full with -radius 0 makes every node a direct peer of the origin, "+
				"so spanMs is pinned near pullsync's ~1s pageTimeout regardless of node count; "+
				"use -topology ring or -topology k-nearest -degree 6 to measure size scaling",
			"topology", string(opts.Base.Topology), "radius", opts.Base.Radius,
		)
	}

	cw := csv.NewWriter(w)
	defer cw.Flush()
	if err := cw.Write(benchHeader()); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("flush header: %w", err)
	}

	total := len(opts.Nodes) * len(opts.Chunks) * opts.Reps
	done := 0
	for _, nodes := range opts.Nodes {
		for _, chunks := range opts.Chunks {
			for rep := 0; rep < opts.Reps; rep++ {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				done++
				logger.Info("bench cell", "nodes", nodes, "chunks", chunks, "rep", rep, "of", total, "done", done)

				row, err := runBenchCell(ctx, opts, nodes, chunks, rep, logger)
				if err != nil {
					return err
				}
				if err := cw.Write(row.csv()); err != nil {
					return fmt.Errorf("write row: %w", err)
				}
				// Flush per row so a long sweep is usable while it runs and a
				// Ctrl-C keeps everything measured so far.
				cw.Flush()
				if err := cw.Error(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// benchStatus derives a cell's verdict from the measurement, not only from the
// wait error. Deriving it from the error alone would report a batch that never
// replicated as "ok" with a span of roughly zero — the origin's own inject is
// then the last put — making the least informative cell in a sweep look like
// the fastest one.
func benchStatus(b sim.Batch, waitErr error) string {
	switch {
	case waitErr != nil && errors.Is(waitErr, context.DeadlineExceeded):
		return "timeout"
	case waitErr != nil:
		// A non-deadline error (an evicted batch, a network closed mid-wait)
		// is a different failure and must not masquerade as a timeout.
		return "error"
	case !b.Settled:
		return "not-settled"
	case b.Replicas == 0:
		return "no-replicas"
	case b.LateReplicas > 0:
		return "truncated"
	}
	return "ok"
}

// runBenchCell builds one network, warms it up, injects a burst, and waits for
// the batch to stop propagating.
func runBenchCell(ctx context.Context, opts benchOptions, nodes, chunks, rep int, logger log.Logger) (benchRow, error) {
	cfg := opts.Base
	cfg.Nodes = nodes
	cfg.Seed = cellSeed(opts.Base.Seed, nodes, chunks, rep)
	cfg.SettleAfter = opts.Settle

	n, err := sim.BuildNetwork(cfg, logger)
	if err != nil {
		return benchRow{}, fmt.Errorf("build network (nodes=%d): %w", nodes, err)
	}
	defer n.Close()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	n.Start(runCtx)

	// Warmup is load-bearing: puller startup plus the first pullsync handshake
	// round, including its ~1s coalescing wait, costs more than most
	// propagation spans. Injecting at t=0 would measure startup and would look
	// like a clean, entirely spurious node-count dependence.
	select {
	case <-time.After(opts.Warmup):
	case <-ctx.Done():
		return benchRow{}, ctx.Err()
	}

	id, _, err := n.Inject(0, chunks, 0, opts.MinPO)
	if err != nil {
		return benchRow{}, fmt.Errorf("inject (nodes=%d chunks=%d): %w", nodes, chunks, err)
	}

	waitCtx, waitCancel := context.WithTimeout(ctx, opts.Timeout)
	defer waitCancel()

	b, waitErr := n.WaitBatch(waitCtx, id)
	if waitErr != nil && ctx.Err() != nil {
		return benchRow{}, ctx.Err() // sweep itself was interrupted
	}
	if waitErr == nil {
		// Keep the network up for one more settle window and re-read the
		// batch. Any replica that lands now proves the quiescence window
		// closed the batch too early, which is otherwise invisible.
		select {
		case <-time.After(opts.Settle):
		case <-ctx.Done():
			return benchRow{}, ctx.Err()
		}
		if nb, ok := n.Batch(id); ok {
			b = nb
		}
	}

	status := benchStatus(b, waitErr)
	switch status {
	case "timeout":
		logger.Warning("bench cell timed out", "nodes", nodes, "chunks", chunks, "rep", rep)
	case "error":
		logger.Warning("bench cell failed", "nodes", nodes, "chunks", chunks, "rep", rep, "error", waitErr)
	case "not-settled":
		logger.Warning("bench cell never settled", "nodes", nodes, "chunks", chunks, "rep", rep)
	case "no-replicas":
		logger.Warning("bench cell saw no replicas; check -radius vs -bench-minpo and topology connectivity",
			"nodes", nodes, "chunks", chunks, "rep", rep)
	case "truncated":
		logger.Warning("bench cell truncated: replicas arrived after the settle window; raise -settle",
			"nodes", nodes, "chunks", chunks, "rep", rep, "late_replicas", b.LateReplicas)
	}

	cc := n.Config()
	return benchRow{
		Nodes: nodes, Chunks: chunks, Rep: rep,
		Topology: string(cc.Topology), Degree: cc.Degree, Radius: cc.Radius,
		Bins: cc.Bins, LatencyMs: cc.Latency.Milliseconds(), MaxPage: cc.MaxPage,
		Clusters: cc.Clusters, Seed: cc.Seed,
		SettleMs: opts.Settle.Milliseconds(), WarmupMs: opts.Warmup.Milliseconds(),
		SpanMs: b.Metrics.SpanMs, InjectMs: b.Metrics.InjectMs, TailMs: b.Metrics.TailMs,
		Replicas: b.Replicas, NodesReached: b.NodesReached, LateReplicas: b.LateReplicas,
		PerDeliveryP50Ms: b.Metrics.PerDeliveryP50Ms,
		PerDeliveryP95Ms: b.Metrics.PerDeliveryP95Ms,
		PerDeliveryMaxMs: b.Metrics.PerDeliveryMaxMs,
		Status:           status,
	}, nil
}
