// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/pullsync"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	"github.com/ethersphere/bee/v2/pkg/soc"
	chunktesting "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/prometheus/client_golang/prometheus"
)

// goroutineLabel is the pprof label that tells the goroutines of the node
// under test apart from those of the other nodes and tests in the process.
const goroutineLabel = "rangesync_node"

var goroutineLabelSeq atomic.Uint64

// labeled runs f with a goroutine label of its own, which every goroutine f
// starts, and theirs, inherits (runtime/pprof), and returns the label.
func labeled(f func()) string {
	id := fmt.Sprintf("n%d", goroutineLabelSeq.Add(1))
	pprof.Do(context.Background(), pprof.Labels(goroutineLabel, id), func(context.Context) { f() })
	return id
}

// reconcilerGoroutines returns the number of goroutines with the label id
// (labeled) that run reconciler code. Goroutines that inherit the label but
// run other code only, such as the peers' stream handlers that streamtest
// starts from this node's sessions, are not counted.
func reconcilerGoroutines(t *testing.T, id string) int {
	t.Helper()
	var b bytes.Buffer
	if err := pprof.Lookup("goroutine").WriteTo(&b, 1); err != nil {
		t.Fatal(err)
	}
	label := fmt.Sprintf("%q:%q", goroutineLabel, id)
	total, count, mine, reconciles := 0, 0, false, false
	flush := func() {
		if mine && reconciles {
			total += count
		}
		count, mine, reconciles = 0, false, false
	}
	for _, line := range strings.Split(b.String(), "\n") {
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "# labels: "):
			mine = strings.Contains(line, label)
		case strings.HasPrefix(line, "#"):
			reconciles = reconciles || strings.Contains(line, "/pkg/rangesync/reconciler.")
		default: // "N @ pcs...", the first line of a record (or the header)
			_, _ = fmt.Sscanf(line, "%d @", &count)
		}
	}
	flush()
	return total
}

// protocolLog is a streamer that records the protocol and stream name of
// every stream its node opens, by peer.
type protocolLog struct {
	p2p.StreamerDisconnecter
	mu      sync.Mutex
	streams map[string][]string // peer -> protocol/stream
}

func newProtocolLog(s p2p.StreamerDisconnecter) *protocolLog {
	return &protocolLog{StreamerDisconnecter: s, streams: make(map[string][]string)}
}

func (l *protocolLog) NewStream(ctx context.Context, addr swarm.Address, h p2p.Headers, protocol, version, stream string) (p2p.Stream, error) {
	s, err := l.StreamerDisconnecter.NewStream(ctx, addr, h, protocol, version, stream)
	if err == nil {
		l.mu.Lock()
		l.streams[addr.ByteString()] = append(l.streams[addr.ByteString()], protocol+"/"+stream)
		l.mu.Unlock()
	}
	return s, err
}

// opened returns how many streams the node opened to peer whose protocol
// and stream name pass match.
func (l *protocolLog) opened(peer swarm.Address, match func(protocol, stream string) bool) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, ps := range l.streams[peer.ByteString()] {
		protocol, stream, _ := strings.Cut(ps, "/")
		if match(protocol, stream) {
			n++
		}
	}
	return n
}

// protocols returns the protocols of every stream the node opened.
func (l *protocolLog) protocols() map[string]bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make(map[string]bool)
	for _, streams := range l.streams {
		for _, ps := range streams {
			protocol, _, _ := strings.Cut(ps, "/")
			out[protocol] = true
		}
	}
	return out
}

// enterThen returns a wrapper that counts the handler's calls in entered,
// then runs each once c is closed, or resets its stream once done is.
func enterThen(entered *atomic.Int32, c, done <-chan struct{}) func(p2p.HandlerFunc) p2p.HandlerFunc {
	wait := waitThen(c, done)
	return func(h p2p.HandlerFunc) p2p.HandlerFunc {
		hw := wait(h)
		return func(ctx context.Context, p p2p.Peer, s p2p.Stream) error {
			entered.Add(1)
			return hw(ctx, p, s)
		}
	}
}

// TestRangesyncOnlyGoroutines: a rangesync-only node runs O(neighbors) sync
// goroutines, not O(bins × neighbors) like the puller's workers, one
// historical and one live per peer and bin (proposal §2.6, P6), and opens no
// pullsync stream. While a round's sessions are held at the peers, the
// reconciler runs its three long-lived goroutines and one per session, and
// opens one session stream per neighbor. Once the node has converged, only
// the three long-lived goroutines remain, whatever the number of neighbors.
func TestRangesyncOnlyGoroutines(t *testing.T) {
	t.Parallel()

	const longLived = 3 // the round loop, the Status gossip loop, the stripe worker
	for _, neighbors := range []int{4, reconciler.MaxInitiatedSessions} {
		t.Run(fmt.Sprintf("%d neighbors", neighbors), func(t *testing.T) {
			t.Parallel()

			// every neighbor holds base and extra; the node lacks extra but
			// holds enough not to bootstrap, so it runs a round session
			// with each neighbor
			base := chunktesting.GenerateTestRandomChunks(16)
			extra := chunktesting.GenerateTestRandomChunks(neighbors)
			gate, done := make(chan struct{}), make(chan struct{})
			var entered atomic.Int32
			peers := make([]*stripeNode, neighbors)
			addrs := make([]swarm.Address, neighbors)
			specs := make(map[string]p2p.ProtocolSpec, neighbors)
			for i := range peers {
				addrs[i] = addr(0xa0, byte(i)) // higher: the node initiates
				peers[i] = newStripeNode(t, streamtest.New(streamtest.WithBaseAddr(addrs[i])), addrs[i], append(base[:len(base):len(base)], extra...)...)
				specs[addrs[i].String()] = wrapStream(peers[i].svc.Protocol(), rangesync.ReconcileStreamName, enterThen(&entered, gate, done))
			}
			var streams *protocolLog
			a := newRoundNode(t, addr(0x10, 0), roundNodeOptions{
				roundInterval: 20 * time.Millisecond,
				chunks:        base,
				streamer: func(s p2p.StreamerDisconnecter) p2p.StreamerDisconnecter {
					streams = newProtocolLog(s)
					return streams
				},
			}, addrs...)
			t.Cleanup(func() { close(done) }) // before the nodes close
			id := labeled(func() { a.start(t, peers, specs) })

			waitFor(t, "a session with every neighbor", func() bool { return int(entered.Load()) == neighbors })
			// every session goroutine is held in its session: at least one
			// per neighbor, and little beyond them and the long-lived ones
			// (a Status send of the gossip, say)
			if got := reconcilerGoroutines(t, id); got < neighbors || got > longLived+2*neighbors {
				t.Fatalf("%d reconciler goroutines with %d sessions in flight, want between %d and %d", got, neighbors, neighbors, longLived+2*neighbors)
			}
			isSession := func(protocol, stream string) bool {
				return protocol == rangesync.ProtocolName && stream == rangesync.ReconcileStreamName
			}
			for _, p := range addrs {
				if n := streams.opened(p, isSession); n != 1 {
					t.Fatalf("%d session streams to %s, want 1", n, p)
				}
			}

			close(gate)
			waitFor(t, "converged", func() bool {
				for _, ch := range extra {
					if !a.has(t, ch) {
						return false
					}
				}
				return true
			})
			waitFor(t, "only the long-lived goroutines", func() bool { return reconcilerGoroutines(t, id) == longLived })
			for protocol := range streams.protocols() {
				if protocol != rangesync.ProtocolName {
					t.Fatalf("the node opened a %s stream", protocol)
				}
			}
		})
	}
}

// pullsyncPeer returns a pullsync-only peer over a mock reserve holding the
// chunks of pages, whose SubscribeBin serves page i to the i-th pull (bin
// IDs from 1 on, across pages).
func pullsyncPeer(t *testing.T, pages ...[]swarm.Chunk) *pullsync.Syncer {
	t.Helper()
	var (
		opts  []mockstorer.Option
		binID uint64
		size  int
	)
	for _, page := range pages {
		offers := make([]*storer.BinC, len(page))
		for i, ch := range page {
			binID++
			id := chunkIDOf(t, ch)
			offers[i] = &storer.BinC{Address: ch.Address(), BatchID: ch.Stamp().BatchID(), BinID: binID, StampHash: id.StampHash}
		}
		opts = append(opts, mockstorer.WithSubscribeResp(offers, nil), mockstorer.WithChunks(page...))
		size = max(size, len(page))
	}
	s := newPullsyncer(nil, mockstorer.NewReserve(opts...), uint64(size))
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// newPullsyncer returns a pullsync instance over store that accepts every
// stamp; maxPage is the most chunks it offers per page.
func newPullsyncer(streamer p2p.Streamer, store storer.Reserve, maxPage uint64) *pullsync.Syncer {
	accept := func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }
	return pullsync.New(streamer, store, func(swarm.Chunk) {}, func(*soc.SOC) {}, accept, log.Noop, maxPage)
}

// rangesyncUnsupported returns a stream error that refuses rangesync streams
// to peer as libp2p does for a protocol the peer does not speak.
func rangesyncUnsupported(peer swarm.Address) func(swarm.Address, string, string, string) error {
	return func(addr swarm.Address, protocol, _, _ string) error {
		if addr.Equal(peer) && protocol == rangesync.ProtocolName {
			return p2p.NewIncompatibleStreamError(errors.New("protocol not supported"))
		}
		return nil
	}
}

// crossProtocolDuplicates returns the cross_protocol_duplicates_total that
// svc reports through its Metrics, as the node registers them.
func crossProtocolDuplicates(t *testing.T, svc *rangesync.Service) float64 {
	t.Helper()
	reg := prometheus.NewRegistry()
	reg.MustRegister(svc.Metrics()...)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range mfs {
		if strings.HasSuffix(mf.GetName(), "_rangesync_cross_protocol_duplicates_total") {
			return mf.GetMetric()[0].GetCounter().GetValue()
		}
	}
	t.Fatal("cross_protocol_duplicates_total not reported")
	return 0
}

// TestDualModeConverges runs a dual-mode node D (rangesync and pullsync over
// one reserve, as with --puller-enable and --rangesync-enable) between a
// rangesync-only peer R and a pullsync-only peer P (proposal §8.3, §13
// coexistence). D's reconciler finds P Unsupported and leaves it to
// pullsync; D pulls P's chunks with pullsync, as its puller does, and
// reconciles with R, so D and R end with the union of the three reserves. A
// chunk x that P and R both hold arrives through both protocols: pullsync
// stores it while D's rangesync fetch of it is in flight, and D reports the
// duplicate in cross_protocol_duplicates_total.
func TestDualModeConverges(t *testing.T) {
	t.Parallel()

	chs := chunktesting.GenerateTestRandomChunks(4)
	x, dOnly, pOnly := chs[0], chs[1], chs[2:]
	base := baseChunks()
	dAddr, rAddr, pAddr := addr(0x10, 0), addr(0xa0, 0), addr(0xc0, 0) // D initiates with R

	p := pullsyncPeer(t, append([]swarm.Chunk{x}, pOnly...))
	var streams *protocolLog
	d := newRoundNode(t, dAddr, roundNodeOptions{
		roundInterval: 20 * time.Millisecond,
		chunks:        append(base[:len(base):len(base)], dOnly),
		streamErr:     rangesyncUnsupported(pAddr),
		streamer: func(s p2p.StreamerDisconnecter) p2p.StreamerDisconnecter {
			streams = newProtocolLog(s)
			return streams
		},
	}, rAddr, pAddr)
	d.routes[pAddr.String()] = p.Protocol()
	dPull := newPullsyncer(streams, d.store, 0)
	t.Cleanup(func() { _ = dPull.Close() })
	// R's rounds run at its start only: D initiates
	r := newRoundNode(t, rAddr, roundNodeOptions{roundInterval: time.Hour, chunks: append(base[:len(base):len(base)], x)}, dAddr)

	// D's only fetch from R asks for x. R serves it once D has pulled from
	// P, which delivers x too.
	var pulled sync.Once
	pull := func(h p2p.HandlerFunc) p2p.HandlerFunc {
		return func(ctx context.Context, peer p2p.Peer, s p2p.Stream) error {
			pulled.Do(func() {
				if _, _, err := dPull.Sync(context.Background(), pAddr, 0, 1); err != nil {
					t.Errorf("pullsync: %v", err)
				}
			})
			return h(ctx, peer, s)
		}
	}
	r.start(t, []*stripeNode{&d.stripeNode}, nil)
	d.start(t, []*stripeNode{&r.stripeNode}, map[string]p2p.ProtocolSpec{
		rAddr.String(): wrapStream(r.svc.Protocol(), rangesync.FetchStreamName, pull),
	})

	union := append(base[:len(base):len(base)], chs...)
	for _, n := range []*roundNode{d, r} {
		waitFor(t, "union", func() bool {
			for _, ch := range union {
				if !n.has(t, ch) {
					return false
				}
			}
			return true
		})
	}

	if got := crossProtocolDuplicates(t, d.svc); got != 1 {
		t.Fatalf("D reports %v cross-protocol duplicates, want 1 (x)", got)
	}
	if got := crossProtocolDuplicates(t, r.svc); got != 0 {
		t.Fatalf("R reports %v cross-protocol duplicates, want 0", got)
	}
	if n := d.deliveriesOf(t, x, rAddr); n != 1 {
		t.Fatalf("R delivered x %d times, want 1", n)
	}
	if s, ok := d.r.PeerState(pAddr); !ok || s != reconciler.StateUnsupported {
		t.Fatalf("D's state of the pullsync-only peer: %v, want %v", s, reconciler.StateUnsupported)
	}
	// rangesync streams go to R only, pullsync streams to P only
	isRangesync := func(protocol, _ string) bool { return protocol == rangesync.ProtocolName }
	isOther := func(protocol, _ string) bool { return protocol != rangesync.ProtocolName }
	if n := streams.opened(pAddr, isRangesync); n != 0 {
		t.Fatalf("D opened %d rangesync streams to the pullsync-only peer", n)
	}
	if n := streams.opened(pAddr, isOther); n == 0 {
		t.Fatal("D opened no pullsync stream to the pullsync-only peer")
	}
	if n := streams.opened(rAddr, isOther); n != 0 {
		t.Fatalf("D opened %d pullsync streams to the rangesync-only peer", n)
	}
	if got := d.r.Score(rAddr); got != 0 {
		t.Fatalf("R scored %d for the duplicate", got)
	}
}

// pullerStandIn stands in for the puller of a dual-mode node: it pulls a
// page from a pullsync peer when the test says so, as the puller's
// historical worker does, and reports the puller's SyncRate, the chunks it
// pulled per second over SyncRateWindow, on the test's clock. pkg/puller
// itself cannot run here: its rate window runs on the real clock, and the
// mock reserve serves one scripted SubscribeBin response per call while a
// puller calls it again and again.
type pullerStandIn struct {
	syncer *pullsync.Syncer
	peer   swarm.Address
	now    func() time.Time

	mu     sync.Mutex
	pulled []pulledPage
}

type pulledPage struct {
	at     time.Time
	chunks int
}

// pull pulls bin 0 from start on and counts the chunks stored.
func (p *pullerStandIn) pull(t *testing.T, start uint64) {
	t.Helper()
	_, n, err := p.syncer.Sync(context.Background(), p.peer, 0, start)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("pulled nothing")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pulled = append(p.pulled, pulledPage{at: p.now(), chunks: n})
}

// SyncRate returns the chunks pulled per second over SyncRateWindow.
func (p *pullerStandIn) SyncRate() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, pg := range p.pulled {
		if p.now().Sub(pg.at) < reconciler.SyncRateWindow {
			n += pg.chunks
		}
	}
	return float64(n) / reconciler.SyncRateWindow.Seconds()
}

// TestDualModeSyncRate: in dual mode the node's SyncRate is the sum of the
// puller's and the reconciler's (pkg/node's combined syncer, §8.4), so it
// reaches 0 only once both have caught up. D bootstraps from the
// rangesync-only peer R while it pulls from the pullsync-only peer P: with
// the puller caught up and the bootstrap still running, and with rangesync
// caught up and a pull still in the window, the sum is above 0.
func TestDualModeSyncRate(t *testing.T) {
	t.Parallel()

	base := chunktesting.GenerateTestRandomChunks(2)
	rOnly := chunktesting.GenerateTestRandomChunks(10) // D's count is below half of R's: it bootstraps
	pages := [][]swarm.Chunk{chunktesting.GenerateTestRandomChunks(2), chunktesting.GenerateTestRandomChunks(2)}
	dAddr, rAddr, pAddr := addr(0x10, 0), addr(0xa0, 0), addr(0xc0, 0)
	clock := &fakeClock{now: time.Unix(1_000_000, 0)}

	p := pullsyncPeer(t, pages...)
	d := newRoundNode(t, dAddr, roundNodeOptions{
		roundInterval: 20 * time.Millisecond,
		chunks:        base,
		now:           clock.Now,
		streamErr:     rangesyncUnsupported(pAddr),
	}, rAddr, pAddr)
	d.routes[pAddr.String()] = p.Protocol()
	puller := &pullerStandIn{syncer: newPullsyncer(d.rec, d.store, 0), peer: pAddr, now: clock.Now}
	t.Cleanup(func() { _ = puller.syncer.Close() })
	r := newRoundNode(t, rAddr, roundNodeOptions{roundInterval: time.Hour, chunks: append(base[:len(base):len(base)], rOnly...)}, dAddr)
	combined := func() float64 { return puller.SyncRate() + d.r.SyncRate() }
	pastWindow := func() { clock.Add(2*reconciler.SyncRateWindow + time.Second) }

	// before the drivers start, the reconciler's start-up guard holds
	if got := combined(); got <= 0 {
		t.Fatalf("combined sync rate %v before the start", got)
	}

	// D's bootstrap fetch waits at R
	fetching, gate, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	t.Cleanup(func() { close(done) }) // before the nodes close
	r.start(t, []*stripeNode{&d.stripeNode}, nil)
	d.start(t, []*stripeNode{&r.stripeNode}, map[string]p2p.ProtocolSpec{
		rAddr.String(): wrapStream(r.svc.Protocol(), rangesync.FetchStreamName, signalThen(fetching, gate, done)),
	})
	waitClosed(t, "the bootstrap fetch", fetching)
	if d.r.StripeRegistry() == 0 {
		t.Fatal("no bootstrap transfer")
	}

	// the puller catches up while rangesync does not
	puller.pull(t, 1)
	if got := puller.SyncRate(); got <= 0 {
		t.Fatalf("puller sync rate %v after a pull", got)
	}
	pastWindow()
	if got := puller.SyncRate(); got != 0 {
		t.Fatalf("puller sync rate %v past the window", got)
	}
	if got := combined(); got < reconciler.StartupGuardRate {
		t.Fatalf("combined sync rate %v with the bootstrap in flight, want at least %v", got, reconciler.StartupGuardRate)
	}

	// rangesync catches up while the puller does not
	close(gate)
	waitFor(t, "bootstrap", func() bool {
		for _, ch := range rOnly {
			if !d.has(t, ch) {
				return false
			}
		}
		return d.r.StripeRegistry() == 0
	})
	pastWindow()
	waitFor(t, "rangesync caught up", func() bool { return d.r.SyncRate() == 0 })
	puller.pull(t, uint64(len(pages[0])+1))
	if got := combined(); got <= 0 {
		t.Fatalf("combined sync rate %v with a pull in the window", got)
	}

	// both caught up; R gets what D pulled through rangesync, which does
	// not count as catch-up
	pastWindow()
	waitFor(t, "R holds the pulled chunks", func() bool {
		for _, ch := range append(pages[0][:len(pages[0]):len(pages[0])], pages[1]...) {
			if !r.has(t, ch) {
				return false
			}
		}
		return true
	})
	if got := combined(); got != 0 {
		t.Fatalf("combined sync rate %v with both caught up", got)
	}
}
