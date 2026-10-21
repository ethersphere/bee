// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"context"
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
	"github.com/ethersphere/bee/v2/pkg/swarm"
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
// each upload it lacks once, and no stream of another protocol is opened;
// with hint(R) and without it (simOptions.NoHints, G6's other setting).
func TestNeighborhoodConverges(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		noHints bool
	}{
		{name: "hints"},
		{name: "no hints", noHints: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := simOptions{Nodes: 5, Replication: 3, Seed: 1, NoHints: tc.noHints}
			res := runNeighborhood(t, o)
			logNeighborhood(t, o, res)
			assertNeighborhood(t, o, res)
		})
	}
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
// and timeouts are on the real clock). A stream open is one round trip, and a
// FullClose waits for the remote's end of the writes, as libp2p's does:
//   - a Status send: the open, the Status, and the handler's close after it
//     read it: 4 legs;
//   - a session: the open, a round trip per pair of messages (at least the
//     opening and its answer), and the closes: the initiator closes once it
//     handled the responder's last Message and its call returns when the
//     responder's close arrives, a round trip later; so at least 6 legs, and
//     a decoded stream without a More 8;
//   - a fetch that stored a chunk: the open, the Want and the Deliveries, at
//     least 4 legs, the first Delivery exactly at the fourth; the holder
//     closes first, so the requester's close returns at once.
//
// Uploads still converge within the bound plus the legs of the slowest chain
// of Status, session and fetch.
func TestNeighborhoodDelay(t *testing.T) {
	t.Parallel()

	const delay = 100 * time.Millisecond
	o := simOptions{Nodes: 5, Replication: 3, Seed: 5, Delay: delay}
	res := runNeighborhood(t, o)
	logNeighborhood(t, o, res)
	// the chain: a Status read by the lacker (the open and one leg), a
	// session that decodes after up to 3 Mores (the open, the opening and
	// the symbols, the items and the end, a round trip per More, the closes)
	// and a fetch (the open, the Want and the deliveries)
	assertNeighborhoodWithin(t, o, res, convergenceBound+(3+(8+2*3)+4)*delay)
	kinds := make(map[string]int)
	for _, sp := range res.Spans {
		if sp.Err != nil {
			continue
		}
		kinds[sp.Kind]++
		if sp.Duration%delay != 0 || sp.Wait%delay != 0 {
			t.Errorf("%s waited %v and took %v, not multiples of the delay %v", sp.Kind, sp.Wait, sp.Duration, delay)
		}
		least := map[string]time.Duration{"status": 4 * delay, "reconcile": 6 * delay}[sp.Kind]
		switch {
		case sp.Kind == "fetch" && sp.Stored > 0:
			least = 4 * delay
			if sp.FirstDelivery != 4*delay {
				t.Errorf("fetch of %d chunks: first delivery after %v, want %v", sp.Stored, sp.FirstDelivery, 4*delay)
			}
		case sp.Kind == "fetch" && sp.Streams > 0:
			least = 2 * delay // the open; reset without a Want when nothing was left to claim
		}
		// a fetch whose IDs were all held or claimed opens no stream
		if sp.Duration < least {
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

// TestNeighborhoodSteady runs the simulation at a steady rate
// (steadyOptions: 24 batches of 20 uploads 1.3 s apart, ~15 uploads per
// second, the load AC-B1 measures) with a 50 ms latency: the batches overlap,
// and each still converges within the bound plus the legs of
// TestNeighborhoodDelay's chain.
func TestNeighborhoodSteady(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("24 batches")
	}

	const delay = 50 * time.Millisecond
	o := steadyOptions(9, 9)
	o.Delay = delay
	res := runNeighborhood(t, o)
	logNeighborhood(t, o, res)
	assertNeighborhoodWithin(t, o, res, convergenceBound+(3+(8+2*3)+4)*delay)
	if got := len(res.Batches); got != 24 {
		t.Fatalf("%d batches, want 24", got)
	}
}

// TestNeighborhoodTeardown checks that a run that ends while a Status send
// waits in its FullClose for the handler's close still tears down. That close
// is a leg on the clock, which no longer moves once the run ended, so the
// harness must end the wait itself (simNeighborhood.close) before it waits for
// the reconcilers. A first run finds the Statuses sent after the convergence;
// a second ends mid-way through the earliest one's FullClose (from its second
// leg to its fourth, see TestNeighborhoodDelay). The harness is
// near-deterministic, not exactly repeatable (which session claims a chunk
// first within an instant is up to the scheduler), so the second run may not
// repeat the first one's timeline: then it tries the next send, up to tries
// runs.
func TestNeighborhoodTeardown(t *testing.T) {
	t.Parallel()

	const (
		delay    = 100 * time.Millisecond
		watchdog = 30 * time.Second // a teardown takes milliseconds
		tries    = 3
	)
	o := simOptions{Nodes: 5, Replication: 3, Seed: 5, Delay: delay}
	res := runNeighborhood(t, o)
	done := o.withDefaults().Warmup + res.Converged
	var sent []time.Duration // when the Statuses sent after the convergence started
	for _, sp := range res.Spans {
		if at := sp.At + sp.Wait; sp.Kind == "status" && sp.Err == nil && at >= done {
			sent = append(sent, at)
		}
	}
	slices.Sort(sent)
	sent = slices.Compact(sent)
	if len(sent) == 0 {
		t.Fatal("no Status sent after the convergence")
	}

	var tails []time.Duration
	for _, at := range sent[:min(tries, len(sent))] {
		o.Tail = at - done + 3*delay
		tails = append(tails, o.Tail)
		nb := newNeighborhood(t, o)
		res := nb.run()
		closed := make(chan struct{})
		go func() {
			defer close(closed)
			nb.close() // again in the cleanup: it returns at once then
		}()
		select {
		case <-closed:
		case <-time.After(watchdog):
			// end the waits on the clock, so that the teardown can end
			// before the test does; the cleanup's close returns at once
			for range 100 {
				nb.clock.advance(nb.clock.Now().Add(simCloseDeadline))
				select {
				case <-closed:
					t.Fatalf("teardown of a run that ended mid-FullClose still waited after %v", watchdog)
				case <-time.After(10 * time.Millisecond):
				}
			}
			t.Fatalf("teardown of a run that ended mid-FullClose still waits after %v and the close deadlines", watchdog)
		}
		// did the run end with a Status send in its FullClose, past its open?
		nb.mu.Lock()
		hit := slices.ContainsFunc(nb.spans[len(res.Spans):], func(sp simSpan) bool {
			return sp.Kind == "status" && sp.Duration >= 2*delay && sp.Duration < 4*delay
		})
		nb.mu.Unlock()
		if hit {
			return
		}
	}
	t.Errorf("no Status send ended in the teardown within its FullClose in %d runs that ended %v after the convergence: "+
		"either each run's timeline differed from the first run's (the harness is near-deterministic, not exactly repeatable), "+
		"or the protocol's timing moved the sends past the close", len(tails), tails)
}

// TestSimClose checks the harness's teardown (simNeighborhood.close) on its
// own: it ends a read and a FullClose parked on the clock, which no longer
// moves, refuses new streams with errSimClosed, and only the first call
// closes: a second returns at once, also while the first still waits.
func TestSimClose(t *testing.T) {
	t.Parallel()

	nb := &simNeighborhood{tb: t, o: simOptions{}.withDefaults()}
	nb.clock = &simClock{now: time.Unix(1_000_000, 0), touch: nb.touch}
	newEnd := func() *simEnd {
		pair := &simPair{stream: rangesync.ReconcileStreamName, toHandler: newSimPipe(nb, false), toInitiator: newSimPipe(nb, false)}
		nb.streams = append(nb.streams, pair)
		return &simEnd{Stream: nopStream{}, in: pair.toInitiator, out: pair.toHandler}
	}
	reader, closer := newEnd(), newEnd()
	read, fullClose := make(chan error, 1), make(chan error, 1)
	go func() {
		_, err := reader.Read(make([]byte, 1)) // nothing arrives
		read <- err
	}()
	go func() { fullClose <- closer.FullClose() }() // the remote never closes
	waitFor(t, "parked read and full close", func() bool { return nb.parked.Load() == 2 })

	// the first close waits for nb.mu, which the test holds; the second
	// returns at once
	nb.mu.Lock()
	first := make(chan struct{})
	go func() {
		defer close(first)
		nb.close()
	}()
	waitFor(t, "first close", nb.closing.Load)
	second := make(chan struct{})
	go func() {
		defer close(second)
		nb.close()
	}()
	select {
	case <-second:
	case <-time.After(10 * time.Second):
		t.Fatal("a second close waits for the first")
	}
	nb.mu.Unlock()
	<-first

	if err := <-read; !errors.Is(err, io.EOF) {
		t.Errorf("parked read after the close: %v, want EOF", err)
	}
	if err := <-fullClose; err != nil {
		t.Errorf("parked full close after the close: %v, want nil (as on the remote's close)", err)
	}
	s := &simStreamer{n: &simNode{nb: nb}}
	if _, err := s.NewStream(context.Background(), swarm.ZeroAddress, nil, rangesync.ProtocolName, rangesync.ProtocolVersion, rangesync.ReconcileStreamName); !errors.Is(err, errSimClosed) {
		t.Errorf("stream opened after the close: %v, want %v", err, errSimClosed)
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
	// parked waits until want readers of nb are parked; t is the subtest's.
	parked := func(t *testing.T, nb *simNeighborhood, want int64) {
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
	// fullClose runs a.FullClose in a goroutine and returns once it is
	// parked; t is the subtest's.
	fullClose := func(t *testing.T, nb *simNeighborhood, a *simEnd) <-chan error {
		t.Helper()
		done := make(chan error, 1)
		go func() { done <- a.FullClose() }()
		parked(t, nb, 1)
		return done
	}
	t.Run("full close waits for the remote's close", func(t *testing.T) {
		t.Parallel()
		nb, a, b := newEnds(0)
		done := fullClose(t, nb, a)
		if got := <-read(b); got != io.EOF.Error() {
			t.Fatalf("read %q, want EOF", got)
		}
		select {
		case err := <-done:
			t.Fatalf("full close returned %v before the remote closed", err)
		default:
		}
		if err := b.FullClose(); err != nil { // a's end of the writes is there
			t.Fatalf("remote's full close: %v", err)
		}
		if err := <-done; err != nil {
			t.Fatalf("full close: %v", err)
		}
		parked(t, nb, 0)
		if _, err := b.Write([]byte("y")); !errors.Is(err, streamtest.ErrStreamClosed) {
			t.Fatalf("write after the full closes: %v", err)
		}
	})
	t.Run("full close gets bytes instead", func(t *testing.T) {
		t.Parallel()
		nb, a, b := newEnds(0)
		done := fullClose(t, nb, a)
		if _, err := b.Write([]byte("y")); err != nil {
			t.Fatal(err)
		}
		if err := <-done; !errors.Is(err, errSimExpectedEOF) {
			t.Fatalf("full close: %v, want %v", err, errSimExpectedEOF)
		}
		if got := <-read(b); got != errSimReset.Error() {
			t.Fatalf("remote read %q, want %v", got, errSimReset)
		}
	})
	t.Run("full close deadline", func(t *testing.T) {
		t.Parallel()
		nb, a, b := newEnds(0)
		done := fullClose(t, nb, a)
		nb.clock.advance(nb.clock.Now().Add(simCloseDeadline))
		if err := <-done; !errors.Is(err, errSimCloseTimeout) {
			t.Fatalf("full close: %v, want %v", err, errSimCloseTimeout)
		}
		if got := <-read(b); got != errSimReset.Error() {
			t.Fatalf("remote read %q, want %v", got, errSimReset)
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
		parked(t, nb, 1)
		if err := a.Reset(); err != nil {
			t.Fatal(err)
		}
		if s := <-got; s != errSimReset.Error() {
			t.Fatalf("read %q after the remote's reset, want %v", s, errSimReset)
		}
		parked(t, nb, 0)
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
		parked(t, nb, 1) // the bytes are on the way
		if at, ok := nb.clock.next(); !ok || at != start.Add(delay) {
			t.Fatalf("next timer %v (%v), want the arrival at %v", at, ok, start.Add(delay))
		}
		nb.clock.advance(start.Add(delay))
		// the reset arrives with the bytes and discards them
		if s := <-got; s != errSimReset.Error() {
			t.Fatalf("read %q, want %v", s, errSimReset)
		}
		parked(t, nb, 0)
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
	// what each side reported (SessionSample): an exact side's difference is
	// what its decoders recovered, a decoder recovers at most one ID per
	// symbol received, and each completed session has one exact sample (the
	// decoding side, or the initiator of a session with no difference)
	for _, sm := range res.PairwiseDiff {
		if ss := sm.Session; sm.At >= res.Batches[0].At && (ss.Exact && ss.Difference != ss.Recovered || ss.Recovered > ss.SymbolsReceived) {
			t.Errorf("node %d at %v: sample %+v", sm.Node, sm.At, ss)
		}
	}
	if exact := res.exactDiffsFrom(res.Batches[0].At); float64(len(exact)) != res.Sessions["ok"] {
		t.Errorf("%d exact pairwise_diff samples, want one per completed session (%v)", len(exact), res.Sessions["ok"])
	}
	if spd, symbols := res.symbolsPerDifference(res.Batches[0].At); symbols == 0 || spd < 1 {
		t.Errorf("%d symbols, %v per difference", symbols, spd)
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
	var meta, metaIn, chunks float64
	deliveries := 0
	for _, n := range res.Nodes {
		meta += n.MetadataOut
		metaIn += n.MetadataIn
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
	// AC-B1's baseline, modeled, not measured: under pullsync every node is
	// offered each upload by each of its N - 1 peers, ~102 B per offer.
	// AC-B1 counts the bytes received; in the closed mesh every byte sent is
	// received, so the sum of bytes_metadata{in} equals that of {out} once
	// the run is quiet (both are logged).
	pullsync := float64(o.Nodes*(o.Nodes-1)) * 102
	perStream := make([]string, 0, len(res.StreamBytes))
	for _, name := range slices.Sorted(maps.Keys(res.StreamBytes)) {
		perStream = append(perStream, fmt.Sprintf("%s %.0f", name, float64(res.StreamBytes[name])/uploads))
	}
	exact := res.exactDiffsFrom(res.Batches[0].At)
	spd, symbols := res.symbolsPerDifference(res.Batches[0].At)
	t.Logf("N=%d p=%d: convergence/rounds per batch %v; %d deliveries, %d duplicates; metadata %.0f B per upload sent, %.0f received (%.0f%% of a modeled pullsync's ~%.0f B), chunk bytes %.0f B per upload; stream bytes per upload: %s",
		o.Nodes, o.Replication, conv, deliveries, res.Duplicates, meta/uploads, metaIn/uploads, 100*metaIn/uploads/pullsync, pullsync, chunks/uploads, strings.Join(perStream, ", "))
	t.Logf("sessions %v; pairwise_diff %d samples (%d zero), p50 %v, p99 %v, exact side %d samples, p50 %v, p99 %v; %d symbols, %.2f per difference; last session %v after convergence; %d steps, %v",
		res.Sessions, len(diffs), zero, quantile(diffs, 0.5), quantile(diffs, 0.99), len(exact), quantile(exact, 0.5), quantile(exact, 0.99),
		symbols, spd, res.LastSession, res.Steps, res.Wall.Round(time.Millisecond))
}
