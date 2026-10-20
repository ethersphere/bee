// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
)

// convergenceBound is the most time an upload pushed to p nodes takes to
// reach every node of a neighborhood, on an otherwise quiet network, and
// convergenceRounds the most rounds a node runs meanwhile.
//
// The bound follows §7.8 and the round scheduler (T4.3). An upload changes the
// root of each of its p holders. Within StatusMinInterval the holder's index
// signals the change (its debounce; a timed round may see it sooner): the
// signal sends the holder's Status to every eligible neighbor, unless the
// per-peer schedule defers a changed summary to StatusMinInterval after the
// last one, and triggers the holder's round, which runs no sooner than
// MinRoundInterval after its last round. So:
//   - a lacker with a higher overlay than the holder gets a session from the
//     holder by StatusMinInterval + MinRoundInterval;
//   - a lacker with a lower overlay has the holder's Status by 2 ×
//     StatusMinInterval, which triggers its own round within MinRoundInterval:
//     a session with the holder by 2 × StatusMinInterval + MinRoundInterval.
//
// In that session the lacker decodes the upload and claims it in its want
// set; sessions with the other holders skip it, so each lacker fetches it once
// (§7.8: one round, no redundant fetch). The adaptive interval does not add to
// the bound, since the root-change and Status triggers do not wait for the
// round timer: at its maximum (30 s) it would. A node's rounds are at least
// MinRoundInterval apart, so it runs at most convergenceBound /
// MinRoundInterval + 1 rounds in the closed window from the upload to its
// convergence (both instants count: the rounds at 0, 5, 10 and 15 s). The
// extra rounds over §7.8's single round are the Status latency (debounce and
// schedule), which §7.8's round model does not count.
//
// The bound is looser than AC-V1's 2 × RoundInterval (10 s at the minimum):
// the derivation above reaches 15 s, when the upload lands on a lower-overlay
// lacker's holders just after their last change sends and rounds.
const (
	convergenceBound  = 2*rangesync.StatusMinInterval + rangesync.MinRoundInterval
	convergenceRounds = int(convergenceBound/rangesync.MinRoundInterval) + 1
)

// TestNeighborhoodConverges runs the neighborhood simulation with N = 5 and
// p = 3: random uploads converge within convergenceBound, every node fetches
// each upload it lacks once, and no stream of another protocol is opened.
func TestNeighborhoodConverges(t *testing.T) {
	t.Parallel()

	o := simOptions{Nodes: 5, Replication: 3, Seed: 1}
	res := runNeighborhood(t, o)
	logNeighborhood(t, o, res)
	assertNeighborhood(t, o, res)
}

// TestNeighborhoodSREP runs the neighborhood simulation at §7.8's sizes,
// N = 9 and N = 17, with p = 3.
func TestNeighborhoodSREP(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("neighborhoods of 9 and 17 nodes")
	}

	for _, n := range []int{9, 17} {
		t.Run(fmt.Sprintf("N=%d", n), func(t *testing.T) {
			t.Parallel()
			o := simOptions{Nodes: n, Replication: 3, Seed: uint64(n)}
			res := runNeighborhood(t, o)
			logNeighborhood(t, o, res)
			assertNeighborhood(t, o, res)
		})
	}
}

// TestNeighborhoodRoundBounds checks that the simulation's bounds of the
// adaptive round interval reach the reconcilers: after the quiet tail every
// node's interval is at the overridden maximum, no node runs two rounds
// closer than the overridden minimum, and an upload converges within the
// bound of the overridden minimum.
//
// The minimum is not below MinRoundInterval: PeerSessionInterval and the
// reconciler's session backoff are tied to it, so a node with shorter rounds
// would trip its peers' session limits in production (the simulation lifts
// them and counts the trips, RateExceeded, which must stay 0).
func TestNeighborhoodRoundBounds(t *testing.T) {
	t.Parallel()

	const lo, hi = 7 * time.Second, 8 * time.Second
	o := simOptions{Nodes: 3, Replication: 1, Seed: 3, Batches: 2, BatchSize: 1, MinRoundInterval: lo, MaxRoundInterval: hi}
	res := runNeighborhood(t, o)
	for i, n := range res.Nodes {
		if n.RoundInterval != hi {
			t.Errorf("node %d: round interval %v after the tail, want %v", i, n.RoundInterval, hi)
		}
		// a late round (a starved goroutine, see Settle) only widens a gap
		if n.Rounds < 2 || n.RoundGap < lo {
			t.Errorf("node %d: %d rounds, at least %v apart, want rounds at least %v apart", i, n.Rounds, n.RoundGap, lo)
		}
	}
	for i, b := range res.Batches {
		if bound := 2*rangesync.StatusMinInterval + lo; b.Convergence > bound {
			t.Errorf("batch %d converged in %v, bound %v", i, b.Convergence, bound)
		}
	}
	if res.RateExceeded != 0 {
		t.Errorf("%d Statuses or sessions over a peer's rate on the fake clock", res.RateExceeded)
	}
}

// TestNeighborhoodDelay runs the simulation with a one-way stream latency on
// the clock. Time passes in a call only by its message legs (the fetch limits
// and timeouts are on the real clock): a stream open is one round trip, a
// completed session at least one more (the opening and its answer) and one
// per further pair of messages, a Status send one leg and a fetch's Want and
// deliveries one leg each. Uploads still converge within the bound plus the
// legs of the slowest chain of Status, session and fetch.
func TestNeighborhoodDelay(t *testing.T) {
	t.Parallel()

	const delay = 100 * time.Millisecond
	o := simOptions{Nodes: 5, Replication: 3, Seed: 5, Delay: delay}
	res := runNeighborhood(t, o)
	logNeighborhood(t, o, res)
	// the chain: a Status (the open and one leg), a session that decodes
	// after up to 3 Mores (the open, the opening and the symbols, the items
	// and the end, a round trip per More) and a fetch (the open, the Want and
	// the deliveries)
	assertNeighborhoodWithin(t, o, res, convergenceBound+(3+(6+2*3)+4)*delay)
	kinds := make(map[string]int)
	for _, sp := range res.Spans {
		if sp.Err != nil {
			continue
		}
		kinds[sp.Kind]++
		if sp.Duration%delay != 0 {
			t.Errorf("%s took %v, not a multiple of the delay %v", sp.Kind, sp.Duration, delay)
		}
		// a fetch whose IDs were all held or claimed opens no stream
		if least := map[string]time.Duration{"status": 2 * delay, "reconcile": 4 * delay}[sp.Kind]; sp.Duration < least {
			t.Errorf("%s took %v, want at least %v", sp.Kind, sp.Duration, least)
		}
		if sp.Kind == "reconcile" && sp.Duration%(2*delay) != 0 {
			t.Errorf("session took %v, not whole round trips of %v", sp.Duration, 2*delay)
		}
	}
	for _, kind := range []string{"status", "reconcile", "fetch"} {
		if kinds[kind] == 0 {
			t.Errorf("no completed %s call", kind)
		}
	}
}

// TestSimStream checks the harness's streams (simEnd, simPipe): the closing
// and reset rules, the delay on the clock, and that a reader waiting for a
// stream is parked (not counted in flight).
func TestSimStream(t *testing.T) {
	t.Parallel()

	newEnds := func(delay time.Duration) (nb *simNeighborhood, a, b *simEnd) {
		nb = &simNeighborhood{o: simOptions{Delay: delay}.withDefaults()}
		nb.clock = &simClock{now: time.Unix(1_000_000, 0), touch: nb.touch}
		ab, ba := newSimPipe(nb, false), newSimPipe(nb, false)
		return nb, &simEnd{Stream: nopStream{}, in: ba, out: ab}, &simEnd{Stream: nopStream{}, in: ab, out: ba}
	}
	// read reads b in a goroutine and returns what it read, once it did.
	read := func(b *simEnd) <-chan string {
		got := make(chan string, 1)
		go func() {
			buf := make([]byte, 16)
			n, err := b.Read(buf)
			if err != nil {
				got <- err.Error()
				return
			}
			got <- string(buf[:n])
		}()
		return got
	}
	parked := func(nb *simNeighborhood, want int64) {
		t.Helper()
		waitFor(t, "parked reader", func() bool { return nb.parked.Load() == want })
	}

	t.Run("close", func(t *testing.T) {
		t.Parallel()
		_, a, b := newEnds(0)
		if _, err := a.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
		if err := a.Close(); err != nil {
			t.Fatal(err)
		}
		if got := <-read(b); got != "x" {
			t.Fatalf("read %q, want x", got)
		}
		if got := <-read(b); got != io.EOF.Error() {
			t.Fatalf("read %q after the close, want EOF", got)
		}
		if _, err := b.Write([]byte("y")); err != nil {
			t.Fatalf("write after the remote's close: %v", err)
		}
	})
	t.Run("full close", func(t *testing.T) {
		t.Parallel()
		_, a, b := newEnds(0)
		if err := a.FullClose(); err != nil {
			t.Fatal(err)
		}
		if got := <-read(b); got != io.EOF.Error() {
			t.Fatalf("read %q, want EOF", got)
		}
		if _, err := b.Write([]byte("y")); !errors.Is(err, streamtest.ErrStreamClosed) {
			t.Fatalf("write after the remote's full close: %v", err)
		}
	})
	t.Run("reset", func(t *testing.T) {
		t.Parallel()
		nb, a, b := newEnds(0)
		if _, err := a.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
		got := read(b) // reads x
		if s := <-got; s != "x" {
			t.Fatalf("read %q, want x", s)
		}
		got = read(b)
		parked(nb, 1)
		if err := a.Reset(); err != nil {
			t.Fatal(err)
		}
		if s := <-got; s != errSimReset.Error() {
			t.Fatalf("read %q after the remote's reset, want %v", s, errSimReset)
		}
		parked(nb, 0)
		if _, err := b.Write([]byte("y")); !errors.Is(err, errSimReset) {
			t.Fatalf("write after the remote's reset: %v, want %v", err, errSimReset)
		}
		if _, err := a.Read(make([]byte, 1)); err == nil {
			t.Fatal("read of a reset end")
		}
	})
	t.Run("delay", func(t *testing.T) {
		t.Parallel()
		const delay = 10 * time.Millisecond
		nb, a, b := newEnds(delay)
		start := nb.clock.Now()
		if _, err := a.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
		if err := a.Reset(); err != nil {
			t.Fatal(err)
		}
		got := read(b)
		parked(nb, 1) // the bytes are on the way
		if at, ok := nb.clock.next(); !ok || at != start.Add(delay) {
			t.Fatalf("next timer %v (%v), want the arrival at %v", at, ok, start.Add(delay))
		}
		nb.clock.advance(start.Add(delay))
		// the reset arrives with the bytes and discards them
		if s := <-got; s != errSimReset.Error() {
			t.Fatalf("read %q, want %v", s, errSimReset)
		}
		parked(nb, 0)
	})
}

// nopStream is a p2p.Stream that does nothing, under a simEnd of TestSimStream.
type nopStream struct{ p2p.Stream }

func (nopStream) Close() error     { return nil }
func (nopStream) FullClose() error { return nil }
func (nopStream) Reset() error     { return nil }

// assertNeighborhood checks the acceptance of T4.6 on res, the result of a
// simulation with options o.
func assertNeighborhood(t *testing.T, o simOptions, res *simResult) {
	t.Helper()
	assertNeighborhoodWithin(t, o, res, convergenceBound)
}

// assertNeighborhoodWithin is assertNeighborhood with the convergence bound
// bound.
func assertNeighborhoodWithin(t *testing.T, o simOptions, res *simResult, bound time.Duration) {
	t.Helper()
	o = o.withDefaults()

	for i, b := range res.Batches {
		if b.Convergence > bound {
			t.Errorf("batch %d converged in %v, bound %v", i, b.Convergence, bound)
		}
		if b.Rounds > convergenceRounds {
			t.Errorf("batch %d took %d rounds of a node, bound %d", i, b.Rounds, convergenceRounds)
		}
	}

	// every node stored each upload it lacked, delivered once
	if res.Duplicates != 0 {
		t.Errorf("%d duplicate rangesync deliveries", res.Duplicates)
	}
	uploads := o.Batches * o.BatchSize
	for i, n := range res.Nodes {
		if lacked := uploads - n.Uploads; n.Stored != lacked || n.Deliveries != lacked {
			t.Errorf("node %d: stored %d, %d deliveries, want the %d uploads it lacked", i, n.Stored, n.Deliveries, lacked)
		}
		// a granted claim whose chunk another fetch stored meanwhile is
		// released unrequested (the reserve check after the claim, T4.4)
		if got := n.WantClaims["granted"]; got < float64(n.Stored) {
			t.Errorf("node %d: want_claims{granted} %v, want at least the %d stored", i, got, n.Stored)
		}
		if got := n.WantClaims["expired"] + n.WantClaims["late"]; got != 0 {
			t.Errorf("node %d: %v expired or late claims", i, got)
		}
	}

	if len(res.Protocols) != 1 || !res.Protocols[rangesync.ProtocolName] {
		t.Errorf("streams of protocols %v, want %s only (no pullsync)", res.Protocols, rangesync.ProtocolName)
	}
	if res.RateExceeded != 0 {
		t.Errorf("%d Statuses or sessions over a peer's rate on the fake clock", res.RateExceeded)
	}
	if got := res.Sessions["error"]; got != 0 {
		t.Errorf("%v sessions failed", got)
	}

	// pairwise_diff: each completed session observed by both sides, below
	// CatchUpMinDiff at this load, and the session differences cover every
	// chunk fetched
	diffs := res.diffsFrom(res.Batches[0].At)
	if want := 2 * res.Sessions["ok"]; float64(len(diffs)) != want {
		t.Errorf("%d pairwise_diff samples, want %v (both sides of each session)", len(diffs), want)
	}
	stored, sum := 0, 0.0
	for _, n := range res.Nodes {
		stored += n.Stored
	}
	for _, d := range diffs {
		sum += d
		if d >= rangesync.CatchUpMinDiff {
			t.Errorf("pairwise_diff sample %v at least CatchUpMinDiff", d)
		}
	}
	if sum < 2*float64(stored) {
		t.Errorf("pairwise_diff samples sum to %v, want at least twice the %d chunks fetched", sum, stored)
	}
	// the histogram the node exposes (Service.Metrics) holds the same samples
	exposed := 0
	for _, n := range res.Nodes {
		exposed += int(n.PairwiseDiff)
	}
	if exposed != len(diffs) {
		t.Errorf("exposed pairwise_diff counts %d samples, want %d", exposed, len(diffs))
	}

	// sessions stop once the uploads converged (AC-C2)
	if quiet := rangesync.StatusMaxInterval + 2*rangesync.MaxRoundInterval; res.LastSession > quiet {
		t.Errorf("a session started %v after convergence, want within %v", res.LastSession, quiet)
	}
}

// logNeighborhood logs the measurements of res.
func logNeighborhood(t *testing.T, o simOptions, res *simResult) {
	t.Helper()
	o = o.withDefaults()
	conv := make([]string, 0, len(res.Batches))
	for _, b := range res.Batches {
		conv = append(conv, fmt.Sprintf("%v/%d", b.Convergence, b.Rounds))
	}
	var meta, chunks float64
	deliveries := 0
	for _, n := range res.Nodes {
		meta += n.MetadataOut
		chunks += n.ChunksOut
		deliveries += n.Deliveries
	}
	uploads := float64(o.Batches * o.BatchSize)
	diffs := res.diffsFrom(res.Batches[0].At)
	zero := 0
	for _, d := range diffs {
		if d == 0 {
			zero++
		}
	}
	// pullsync: every node is offered each upload by each of its N - 1 peers,
	// ~102 B per offer (AC-B1)
	pullsync := float64(o.Nodes*(o.Nodes-1)) * 102
	perStream := make([]string, 0, len(res.StreamBytes))
	for _, name := range slices.Sorted(maps.Keys(res.StreamBytes)) {
		perStream = append(perStream, fmt.Sprintf("%s %.0f", name, float64(res.StreamBytes[name])/uploads))
	}
	exact := res.exactDiffsFrom(res.Batches[0].At)
	spd, symbols := res.symbolsPerDifference(res.Batches[0].At)
	t.Logf("N=%d p=%d: convergence/rounds per batch %v; %d deliveries, %d duplicates; metadata %.0f B per upload (%.0f%% of pullsync's ~%.0f B), chunk bytes %.0f B per upload; stream bytes per upload: %s",
		o.Nodes, o.Replication, conv, deliveries, res.Duplicates, meta/uploads, 100*meta/uploads/pullsync, pullsync, chunks/uploads, strings.Join(perStream, ", "))
	t.Logf("sessions %v; pairwise_diff %d samples (%d zero), p50 %v, p99 %v, exact side %d samples, p50 %v, p99 %v; %d symbols, %.2f per difference; last session %v after convergence; %d steps, %v",
		res.Sessions, len(diffs), zero, quantile(diffs, 0.5), quantile(diffs, 0.99), len(exact), quantile(exact, 0.5), quantile(exact, 0.99),
		symbols, spd, res.LastSession, res.Steps, res.Wall.Round(time.Millisecond))
}
