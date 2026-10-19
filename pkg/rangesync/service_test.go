// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/postage"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/storer"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	ggio "github.com/gogo/protobuf/io"
)

// serviceLeafBits is the index leaf depth of the services in these tests.
const serviceLeafBits = 8

// recordingScorer is a rangesync.PeerScorer that records its calls.
type recordingScorer struct {
	mu       sync.Mutex
	fails    map[string][]error
	succeeds map[string]int
}

func newRecordingScorer() *recordingScorer {
	return &recordingScorer{fails: make(map[string][]error), succeeds: make(map[string]int)}
}

func (s *recordingScorer) Fail(peer swarm.Address, reason error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fails[peer.ByteString()] = append(s.fails[peer.ByteString()], reason)
}

func (s *recordingScorer) Succeed(peer swarm.Address) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.succeeds[peer.ByteString()]++
}

func (s *recordingScorer) get(peer swarm.Address) (fails []error, succeeds int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.fails[peer.ByteString()]), s.succeeds[peer.ByteString()]
}

// testNode is a rangesync service over a mock reserve.
type testNode struct {
	addr   swarm.Address
	store  *mockstorer.ReserveStore
	rec    *streamtest.RecorderDisconnecter
	svc    *rangesync.Service
	scorer *recordingScorer

	mu    sync.Mutex
	wants map[string][]storer.ChunkID // responder wants by peer
}

func newTestNode(tb testing.TB, chunks ...swarm.Chunk) *testNode {
	tb.Helper()
	return newTestNodeOpts(tb, nodeOpts{}, chunks...)
}

// nodeOpts configures the fetch side of a testNode.
type nodeOpts struct {
	// validStamp validates delivered stamps; nil accepts every stamp.
	validStamp postage.ValidStampFn
	// outside reports addresses outside the storage radius; nil: none.
	outside func(swarm.Address) bool
	// fetchTimeout is Options.FetchTimeout.
	fetchTimeout time.Duration
	// streamErr is the recorder's stream error; nil: none.
	streamErr func(swarm.Address, string, string, string) error
}

// radiusStore is a mock reserve whose storage radius check is outside.
type radiusStore struct {
	*mockstorer.ReserveStore
	outside func(swarm.Address) bool
}

func (s radiusStore) IsWithinStorageRadius(addr swarm.Address) bool {
	return s.outside == nil || !s.outside(addr)
}

func newTestNodeOpts(tb testing.TB, o nodeOpts, chunks ...swarm.Chunk) *testNode {
	tb.Helper()
	if o.validStamp == nil {
		o.validStamp = func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }
	}
	n := &testNode{
		addr:   swarm.RandAddress(tb),
		store:  mockstorer.NewReserve(mockstorer.WithAddrIndex(), mockstorer.WithChunks(chunks...)),
		scorer: newRecordingScorer(),
		wants:  make(map[string][]storer.ChunkID),
	}
	recOpts := []streamtest.Option{streamtest.WithBaseAddr(n.addr)}
	if o.streamErr != nil {
		recOpts = append(recOpts, streamtest.WithStreamError(o.streamErr))
	}
	n.rec = streamtest.NewRecorderDisconnecter(streamtest.New(recOpts...))
	n.svc = rangesync.New(n.rec, radiusStore{ReserveStore: n.store, outside: o.outside}, n.addr, nil, nil, o.validStamp, log.Noop, rangesync.Options{
		Scorer:       n.scorer,
		Wants:        n.addWants,
		LeafBits:     serviceLeafBits,
		FetchTimeout: o.fetchTimeout,
	})
	tb.Cleanup(func() {
		if err := n.svc.Close(); err != nil {
			tb.Error(err)
		}
	})
	return n
}

func (n *testNode) addWants(peer swarm.Address, wants []storer.ChunkID) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.wants[peer.ByteString()] = append(n.wants[peer.ByteString()], wants...)
}

func (n *testNode) responderWants(peer swarm.Address) []storer.ChunkID {
	n.mu.Lock()
	defer n.mu.Unlock()
	return slices.Clone(n.wants[peer.ByteString()])
}

// start starts the service and waits for its index.
func (n *testNode) start(tb testing.TB) {
	tb.Helper()
	if err := n.svc.Start(); err != nil {
		tb.Fatal(err)
	}
	waitFor(tb, "index", func() bool {
		_, err := n.svc.LocalStatus()
		return err == nil
	})
}

// connect lets a and b open streams to each other and exchanges their status.
func connect(t *testing.T, a, b *testNode) {
	t.Helper()
	a.rec.SetProtocols(b.svc.Protocol())
	b.rec.SetProtocols(a.svc.Protocol())
	for _, x := range [][2]*testNode{{a, b}, {b, a}} {
		if err := x[0].svc.SendStatus(context.Background(), x[1].addr); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "status", func() bool {
			_, ok := x[1].svc.LastStatus(x[0].addr)
			return ok
		})
	}
}

// stampedChunk returns a chunk at a random address with a random stamp. The
// mock does not validate chunks.
func stampedChunk(tb testing.TB) swarm.Chunk {
	tb.Helper()
	return swarm.NewChunk(swarm.RandAddress(tb), []byte{1}).WithStamp(postagetesting.MustNewStamp())
}

func stampedChunks(tb testing.TB, n int) []swarm.Chunk {
	tb.Helper()
	chs := make([]swarm.Chunk, n)
	for i := range chs {
		chs[i] = stampedChunk(tb)
	}
	return chs
}

func chunkIDOf(tb testing.TB, ch swarm.Chunk) storer.ChunkID {
	tb.Helper()
	sh, err := ch.Stamp().Hash()
	if err != nil {
		tb.Fatal(err)
	}
	return storer.ChunkID{Address: ch.Address(), StampHash: sh}
}

func assertChunkIDs(t *testing.T, got []storer.ChunkID, want ...storer.ChunkID) {
	t.Helper()
	key := func(c storer.ChunkID) string { return c.Address.ByteString() + string(c.StampHash) }
	g := make([]string, len(got))
	for i, c := range got {
		if c.BatchID != nil {
			t.Errorf("want %s has a batch ID", c.Address)
		}
		g[i] = key(c)
	}
	w := make([]string, len(want))
	for i, c := range want {
		w[i] = key(c)
	}
	slices.Sort(g)
	slices.Sort(w)
	if !slices.Equal(g, w) {
		t.Fatalf("got %d wants, want %d (%v, %v)", len(g), len(w), got, want)
	}
}

// reconcileMessages returns the Messages the initiator a sent to b and those it
// received on its reconcile streams to b.
func reconcileMessages(t *testing.T, a *testNode, b swarm.Address) (sent, received []*pb.Message) {
	t.Helper()
	records, err := a.rec.Records(b, rangesync.ProtocolName, rangesync.ProtocolVersion, rangesync.ReconcileStreamName)
	if err != nil {
		t.Fatal(err)
	}
	read := func(data []byte) []*pb.Message {
		// up to MaxMessageBytes: protobuf.ReadMessages has a smaller limit
		r := ggio.NewDelimitedReader(bytes.NewReader(data), rangesync.MaxMessageBytes)
		var out []*pb.Message
		for {
			m := new(pb.Message)
			if err := r.ReadMsg(m); errors.Is(err, io.EOF) {
				return out
			} else if err != nil {
				t.Fatal(err)
			}
			out = append(out, m)
		}
	}
	for _, r := range records {
		sent = append(sent, read(r.In())...)
		received = append(received, read(r.Out())...)
	}
	return sent, received
}

func TestStatus(t *testing.T) {
	t.Parallel()

	a := newTestNode(t, stampedChunks(t, 10)...)
	b := newTestNode(t)
	a.start(t)
	b.start(t)
	connect(t, a, b)

	want, err := a.svc.LocalStatus()
	if err != nil {
		t.Fatal(err)
	}
	if want.Summary.Count != 10 || want.Mapping != fingerprint.DefaultMapperID || want.LeafBits != serviceLeafBits || want.Root.Bits != 0 {
		t.Fatalf("local status: %+v", want)
	}
	got, ok := b.svc.LastStatus(a.addr)
	if !ok {
		t.Fatal("no status")
	}
	if !got.Root.Equal(want.Root) || got.Summary != want.Summary || got.LeafBits != want.LeafBits || got.Mapping != want.Mapping {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if _, ok := b.svc.LastStatus(swarm.RandAddress(t)); ok {
		t.Fatal("status of an unknown peer")
	}
	if got := b.svc.MappingMismatches(); got != 0 {
		t.Fatalf("mapping mismatches: %v", got)
	}
}

func TestStatusNotReady(t *testing.T) {
	t.Parallel()

	a := newTestNode(t)
	b := newTestNode(t)
	connect0(a, b)
	// a is not started: it has no index yet
	if err := a.svc.SendStatus(context.Background(), b.addr); !errors.Is(err, fingerprint.ErrBusy) {
		t.Fatalf("got %v, want %v", err, fingerprint.ErrBusy)
	}
}

func TestStatusProtocolUnsupported(t *testing.T) {
	t.Parallel()

	a := newTestNodeOpts(t, nodeOpts{streamErr: func(swarm.Address, string, string, string) error {
		return p2p.NewIncompatibleStreamError(errors.New("protocols not supported"))
	}})
	a.start(t)
	err := a.svc.SendStatus(context.Background(), swarm.RandAddress(t))
	if !errors.Is(err, rangesync.ErrProtocolUnsupported) {
		t.Fatalf("got %v, want %v", err, rangesync.ErrProtocolUnsupported)
	}
}

// connect0 lets a and b open streams to each other without a status exchange.
func connect0(a, b *testNode) {
	a.rec.SetProtocols(b.svc.Protocol())
	b.rec.SetProtocols(a.svc.Protocol())
}

// rawPeer opens streams to a service from address addr, with no service of
// its own. It counts the resets of the server's handlers.
type rawPeer struct {
	addr   swarm.Address
	rec    *streamtest.Recorder
	resets atomic.Int32
}

func newRawPeer(t *testing.T, server *testNode) *rawPeer {
	t.Helper()
	p := &rawPeer{addr: swarm.RandAddress(t)}
	countReset := func(h p2p.HandlerFunc) p2p.HandlerFunc {
		return func(ctx context.Context, peer p2p.Peer, s p2p.Stream) error {
			return h(ctx, peer, &resetRecordingStream{Stream: s, resets: &p.resets})
		}
	}
	p.rec = streamtest.New(
		streamtest.WithBaseAddr(p.addr),
		streamtest.WithProtocols(server.svc.Protocol()),
		streamtest.WithMiddlewares(countReset),
	)
	return p
}

func (p *rawPeer) open(t *testing.T, name string) p2p.Stream {
	t.Helper()
	s, err := p.rec.NewStream(context.Background(), swarm.ZeroAddress, nil, rangesync.ProtocolName, rangesync.ProtocolVersion, name)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// records waits for the server's handlers of the stream name and returns
// their records.
func (p *rawPeer) records(t *testing.T, name string) []*streamtest.Record {
	t.Helper()
	records, err := p.rec.Records(swarm.ZeroAddress, rangesync.ProtocolName, rangesync.ProtocolVersion, name)
	if err != nil {
		t.Fatal(err)
	}
	return records
}

// sendStatus sends st on a status stream and waits for the handler.
func (p *rawPeer) sendStatus(t *testing.T, st *pb.Status) error {
	t.Helper()
	s := p.open(t, rangesync.StatusStreamName)
	if err := protobuf.NewWriter(s).WriteMsg(st); err != nil {
		t.Fatal(err)
	}
	_ = s.FullClose()
	records := p.records(t, rangesync.StatusStreamName)
	return records[len(records)-1].Err()
}

func peerStatus(mapping uint32) *pb.Status {
	return rangesync.EncodeStatus(rangesync.Status{
		Root:     fingerprint.NewRange(swarm.ZeroAddress, 0),
		LeafBits: serviceLeafBits,
		Mapping:  mapping,
	})
}

func assertDisconnect(t *testing.T, err error) {
	t.Helper()
	var de *p2p.DisconnectError
	if !errors.As(err, &de) {
		t.Fatalf("handler error %v is not a disconnect error", err)
	}
	if !errors.Is(err, rangesync.ErrInvalidMessage) {
		t.Fatalf("handler error %v does not wrap %v", err, rangesync.ErrInvalidMessage)
	}
}

func assertFailed(t *testing.T, s *recordingScorer, peer swarm.Address) {
	t.Helper()
	fails, succeeds := s.get(peer)
	if len(fails) != 1 || !errors.Is(fails[0], rangesync.ErrInvalidMessage) {
		t.Fatalf("scorer fails: %v", fails)
	}
	if succeeds != 0 {
		t.Fatalf("scorer succeeds: %d", succeeds)
	}
}

func TestStatusInvalid(t *testing.T) {
	t.Parallel()

	bad := peerStatus(fingerprint.DefaultMapperID)
	bad.Root.Prefix = []byte{0} // not ceil(0/8) bytes
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"non-canonical root", mustMarshalDelimited(t, bad)},
		{"undecodable", []byte{3, 0xff, 0xff, 0xff}},
		{"oversize", binary.AppendUvarint(nil, 1<<20)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := newTestNode(t)
			b.start(t)
			p := newRawPeer(t, b)
			s := p.open(t, rangesync.StatusStreamName)
			if _, err := s.Write(tc.data); err != nil {
				t.Fatal(err)
			}
			_ = s.Close()
			records := p.records(t, rangesync.StatusStreamName)
			assertDisconnect(t, records[0].Err())
			assertFailed(t, b.scorer, p.addr)
			if p.resets.Load() == 0 {
				t.Fatal("stream not reset")
			}
			if _, ok := b.svc.LastStatus(p.addr); ok {
				t.Fatal("invalid status recorded")
			}
		})
	}
}

func mustMarshalDelimited(tb testing.TB, m protobuf.Message) []byte {
	tb.Helper()
	var buf bytes.Buffer
	if err := protobuf.NewWriter(&buf).WriteMsg(m); err != nil {
		tb.Fatal(err)
	}
	return buf.Bytes()
}

func TestMappingMismatch(t *testing.T) {
	t.Parallel()

	b := newTestNode(t, stampedChunks(t, 5)...)
	b.start(t)
	p := newRawPeer(t, b)
	if err := p.sendStatus(t, peerStatus(fingerprint.DefaultMapperID+1)); err != nil {
		t.Fatal(err)
	}
	if got := b.svc.MappingMismatches(); got != 1 {
		t.Fatalf("mapping mismatches: got %v, want 1", got)
	}
	st, ok := b.svc.LastStatus(p.addr)
	if !ok || st.Mapping != fingerprint.DefaultMapperID+1 {
		t.Fatalf("status: %+v %v", st, ok)
	}

	// b does not open a session to p
	if _, err := b.svc.Reconcile(context.Background(), p.addr); !errors.Is(err, rangesync.ErrMappingMismatch) {
		t.Fatalf("reconcile: got %v, want %v", err, rangesync.ErrMappingMismatch)
	}

	// nor serve one: the stream is reset, the peer is not disconnected or scored
	s := p.open(t, rangesync.ReconcileStreamName)
	w := protobuf.NewWriter(s)
	if err := w.WriteMsg(msg(fpPart(fingerprint.NewRange(swarm.ZeroAddress, 0), fingerprint.Summary{}))); err != nil {
		t.Fatal(err)
	}
	records := p.records(t, rangesync.ReconcileStreamName)
	err := records[0].Err()
	if !errors.Is(err, rangesync.ErrMappingMismatch) {
		t.Fatalf("handler: got %v, want %v", err, rangesync.ErrMappingMismatch)
	}
	var de *p2p.DisconnectError
	if errors.As(err, &de) {
		t.Fatal("peer disconnected")
	}
	if fails, _ := b.scorer.get(p.addr); len(fails) != 0 {
		t.Fatalf("peer scored: %v", fails)
	}
	_ = s.FullClose()
}

// TestStatusRateLimit: a peer that sends Statuses faster than
// PeerStatusBurst, then one per PeerStatusInterval, has the excess dropped
// unread (proposal §9, status spam): not recorded, the stream reset, the
// peer neither disconnected nor scored. A disconnect clears its limit.
func TestStatusRateLimit(t *testing.T) {
	t.Parallel()

	b := newTestNode(t)
	b.start(t)
	p := newRawPeer(t, b)
	withCount := func(n uint64) *pb.Status {
		return rangesync.EncodeStatus(rangesync.Status{
			Root:     fingerprint.NewRange(swarm.ZeroAddress, 0),
			Summary:  fingerprint.Summary{Count: n},
			LeafBits: serviceLeafBits,
			Mapping:  fingerprint.DefaultMapperID,
		})
	}
	lastCount := func() uint64 {
		t.Helper()
		st, ok := b.svc.LastStatus(p.addr)
		if !ok {
			t.Fatal("no status")
		}
		return st.Summary.Count
	}

	// the burst is recorded, back to back
	for i := range uint64(rangesync.PeerStatusBurst) {
		if err := p.sendStatus(t, withCount(i+1)); err != nil {
			t.Fatalf("status %d: %v", i+1, err)
		}
		if got := lastCount(); got != i+1 {
			t.Fatalf("status %d: recorded count %d", i+1, got)
		}
	}

	// the next one is over the rate: the handler resets the stream, which
	// may beat the write, so write errors are not checked
	s := p.open(t, rangesync.StatusStreamName)
	_ = protobuf.NewWriter(s).WriteMsg(withCount(100))
	_ = s.FullClose()
	records := p.records(t, rangesync.StatusStreamName)
	err := records[len(records)-1].Err()
	if err == nil {
		t.Fatal("status over the rate accepted")
	}
	var de *p2p.DisconnectError
	if errors.As(err, &de) {
		t.Fatalf("peer disconnected: %v", err)
	}
	if p.resets.Load() == 0 {
		t.Fatal("stream not reset")
	}
	if got := lastCount(); got != rangesync.PeerStatusBurst {
		t.Fatalf("recorded count %d after a status over the rate, want %d", got, rangesync.PeerStatusBurst)
	}
	if fails, _ := b.scorer.get(p.addr); len(fails) != 0 {
		t.Fatalf("peer scored: %v", fails)
	}
	if got := b.svc.StatusRateLimited(); got != 1 {
		t.Fatalf("status_rate_limited_total %v, want 1", got)
	}

	// a disconnect forgets the peer's status (kept in memory only) and
	// clears its limit
	if err := b.svc.Protocol().DisconnectIn(p2p.Peer{Address: p.addr}); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.svc.LastStatus(p.addr); ok {
		t.Fatal("status kept after a disconnect")
	}
	if err := p.sendStatus(t, withCount(200)); err != nil {
		t.Fatalf("status after a reconnect: %v", err)
	}
	if got := lastCount(); got != 200 {
		t.Fatalf("recorded count %d after a reconnect, want 200", got)
	}
}

func TestReconcileEqual(t *testing.T) {
	t.Parallel()

	chs := stampedChunks(t, 300)
	a := newTestNode(t, chs...)
	b := newTestNode(t, chs...)
	a.start(t)
	b.start(t)
	connect(t, a, b)

	wants, err := a.svc.Reconcile(context.Background(), b.addr)
	if err != nil {
		t.Fatal(err)
	}
	if len(wants) != 0 {
		t.Fatalf("got %d wants", len(wants))
	}

	// one round trip: the opening and an empty answer
	sent, received := reconcileMessages(t, a, b.addr)
	if len(sent) != 1 || len(received) != 1 || len(received[0].Parts) != 0 || received[0].Busy {
		t.Fatalf("sent %v, received %v", sent, received)
	}
	if len(b.responderWants(a.addr)) != 0 {
		t.Fatal("responder wants")
	}
	for _, x := range [][2]*testNode{{a, b}, {b, a}} {
		if fails, succeeds := x[0].scorer.get(x[1].addr); len(fails) != 0 || succeeds != 1 {
			t.Fatalf("scorer: %v fails, %d succeeds", fails, succeeds)
		}
	}
}

func TestReconcileSmallDiff(t *testing.T) {
	t.Parallel()

	common := stampedChunks(t, 300)
	onlyA := stampedChunks(t, 3)
	onlyB := stampedChunks(t, 2)
	a := newTestNode(t, append(slices.Clone(common), onlyA...)...)
	b := newTestNode(t, append(slices.Clone(common), onlyB...)...)
	a.start(t)
	b.start(t)
	connect(t, a, b)

	wants, err := a.svc.Reconcile(context.Background(), b.addr)
	if err != nil {
		t.Fatal(err)
	}
	assertChunkIDs(t, wants, chunkIDOf(t, onlyB[0]), chunkIDOf(t, onlyB[1]))

	// the responder's handler has returned: Records waits for it
	reconcileMessages(t, a, b.addr)
	assertChunkIDs(t, b.responderWants(a.addr), chunkIDOf(t, onlyA[0]), chunkIDOf(t, onlyA[1]), chunkIDOf(t, onlyA[2]))
	for _, x := range [][2]*testNode{{a, b}, {b, a}} {
		if fails, succeeds := x[0].scorer.get(x[1].addr); len(fails) != 0 || succeeds != 1 {
			t.Fatalf("scorer: %v fails, %d succeeds", fails, succeeds)
		}
	}
}

func TestReconcileNoStatus(t *testing.T) {
	t.Parallel()

	a := newTestNode(t)
	b := newTestNode(t)
	a.start(t)
	b.start(t)
	connect0(a, b)
	if _, err := a.svc.Reconcile(context.Background(), b.addr); !errors.Is(err, rangesync.ErrNoStatus) {
		t.Fatalf("got %v, want %v", err, rangesync.ErrNoStatus)
	}
}

// openSession opens a reconcile session from p to its server with an opening
// that differs from the server's root, and reads the server's answer: the
// session then waits for p's next message. The stream is closed at cleanup.
func openSession(t *testing.T, p *rawPeer) p2p.Stream {
	t.Helper()
	s := p.open(t, rangesync.ReconcileStreamName)
	t.Cleanup(func() { _ = s.Reset() })
	open := fingerprint.Summary{Count: 1, Digest: [32]byte{1}}
	if err := protobuf.NewWriter(s).WriteMsg(msg(fpPart(fingerprint.NewRange(swarm.ZeroAddress, 0), open))); err != nil {
		t.Fatal(err)
	}
	var answer pb.Message
	if err := protobuf.NewReader(s).ReadMsg(&answer); err != nil {
		t.Fatal(err)
	}
	if answer.Busy || len(answer.Parts) == 0 {
		t.Fatalf("answer: %v", &answer)
	}
	return s
}

func TestReconcileBusy(t *testing.T) {
	t.Parallel()

	assertBusy := func(t *testing.T, a, b *testNode) {
		t.Helper()
		_, err := a.svc.Reconcile(context.Background(), b.addr)
		if !errors.Is(err, rangesync.ErrPeerBusy) {
			t.Fatalf("got %v, want %v", err, rangesync.ErrPeerBusy)
		}
		_, received := reconcileMessages(t, a, b.addr)
		if len(received) != 1 || !received[0].Busy || len(received[0].Parts) != 0 {
			t.Fatalf("received %v", received)
		}
		for _, x := range [][2]*testNode{{a, b}, {b, a}} {
			if fails, succeeds := x[0].scorer.get(x[1].addr); len(fails) != 0 || succeeds != 0 {
				t.Fatalf("scorer: %v fails, %d succeeds", fails, succeeds)
			}
		}
	}

	t.Run("address index filling", func(t *testing.T) {
		t.Parallel()
		a := newTestNode(t, stampedChunk(t))
		b := newTestNode(t, stampedChunk(t))
		a.start(t)
		b.start(t)
		connect(t, a, b)
		b.store.SetAddrIndexReady(false)
		assertBusy(t, a, b)
	})

	t.Run("index not built", func(t *testing.T) {
		t.Parallel()
		a := newTestNode(t, stampedChunk(t))
		b := newTestNode(t, stampedChunk(t))
		a.start(t)
		connect0(a, b)
		if err := a.svc.SendStatus(context.Background(), b.addr); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "status", func() bool { _, ok := b.svc.LastStatus(a.addr); return ok })
		// a needs a status of b to open: send one from a peer with b's address
		p := &rawPeer{addr: b.addr, rec: streamtest.New(streamtest.WithBaseAddr(b.addr), streamtest.WithProtocols(a.svc.Protocol()))}
		if err := p.sendStatus(t, peerStatus(fingerprint.DefaultMapperID)); err != nil {
			t.Fatal(err)
		}
		assertBusy(t, a, b)
	})

	t.Run("no status of the initiator", func(t *testing.T) {
		t.Parallel()
		a := newTestNode(t, stampedChunk(t))
		b := newTestNode(t, stampedChunk(t))
		a.start(t)
		b.start(t)
		connect0(a, b)
		if err := b.svc.SendStatus(context.Background(), a.addr); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "status", func() bool { _, ok := a.svc.LastStatus(b.addr); return ok })
		assertBusy(t, a, b)
	})

	t.Run("session of the peer running", func(t *testing.T) {
		t.Parallel()
		a := newTestNode(t, stampedChunks(t, 20)...)
		b := newTestNode(t, stampedChunks(t, 20)...)
		a.start(t)
		b.start(t)
		connect(t, a, b)
		// a raw peer with a's address holds a session
		p := &rawPeer{addr: a.addr, rec: streamtest.New(streamtest.WithBaseAddr(a.addr), streamtest.WithProtocols(b.svc.Protocol()))}
		openSession(t, p)
		assertBusy(t, a, b)
	})

	t.Run("node-wide cap", func(t *testing.T) {
		t.Parallel()
		a := newTestNode(t, stampedChunks(t, 20)...)
		b := newTestNode(t, stampedChunks(t, 20)...)
		a.start(t)
		b.start(t)
		connect(t, a, b)
		for range rangesync.MaxConcurrentSessions {
			p := newRawPeer(t, b)
			if err := p.sendStatus(t, peerStatus(fingerprint.DefaultMapperID)); err != nil {
				t.Fatal(err)
			}
			openSession(t, p)
		}
		assertBusy(t, a, b)
	})
}

func TestReconcileStreamLimit(t *testing.T) {
	t.Parallel()

	b := newTestNode(t, stampedChunks(t, 20)...)
	b.start(t)
	p := newRawPeer(t, b)
	if err := p.sendStatus(t, peerStatus(fingerprint.DefaultMapperID)); err != nil {
		t.Fatal(err)
	}
	session := openSession(t, p)
	// the other streams wait in the handler for their opening, except the
	// one over the limit, which is reset
	streams := make([]p2p.Stream, rangesync.MaxPeerReconcileStreams)
	ended := make(chan struct{}, len(streams))
	for i := range streams {
		streams[i] = p.open(t, rangesync.ReconcileStreamName)
		go func() {
			_, _ = io.ReadAll(streams[i])
			ended <- struct{}{}
		}()
	}
	select {
	case <-ended:
	case <-time.After(10 * time.Second):
		t.Fatal("no stream reset")
	}
	_ = session.Reset()
	for _, s := range streams {
		_ = s.Reset()
	}
	for range len(streams) - 1 {
		<-ended
	}

	disconnects := 0
	for _, r := range p.records(t, rangesync.ReconcileStreamName) {
		var de *p2p.DisconnectError
		if errors.As(r.Err(), &de) {
			disconnects++
		}
	}
	if disconnects != 1 {
		t.Fatalf("got %d disconnects, want 1", disconnects)
	}
}

func TestReconcileInvalidFromInitiator(t *testing.T) {
	t.Parallel()

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	differs := fingerprint.Summary{Count: 1, Digest: [32]byte{1}}
	nonCanonical := msg(fpPart(root, differs))
	nonCanonical.Parts[0].GetFP().Range.Prefix = []byte{0}
	for _, tc := range []struct {
		name string
		// first is written raw; second, if set, after the server's answer
		first  []byte
		second *pb.Message
	}{
		{name: "undecodable", first: []byte{3, 0xff, 0xff, 0xff}},
		{name: "oversize", first: binary.AppendUvarint(nil, rangesync.MaxMessageBytes+1)},
		{name: "non-canonical range", first: mustMarshalDelimited(t, nonCanonical)},
		{name: "busy from the initiator", first: mustMarshalDelimited(t, &pb.Message{Parts: msg(fpPart(root, differs)).Parts, Busy: true})},
		{name: "items in the opening", first: mustMarshalDelimited(t, msg(itemsPart(rangesync.Items{Range: root})))},
		{
			name:   "unsolicited fingerprint",
			first:  mustMarshalDelimited(t, msg(fpPart(root, differs))),
			second: msg(fpPart(rangeOf("1"), differs)),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := newTestNode(t, stampedChunks(t, 20)...)
			b.start(t)
			p := newRawPeer(t, b)
			if err := p.sendStatus(t, peerStatus(fingerprint.DefaultMapperID)); err != nil {
				t.Fatal(err)
			}
			s := p.open(t, rangesync.ReconcileStreamName)
			if _, err := s.Write(tc.first); err != nil {
				t.Fatal(err)
			}
			if tc.second != nil {
				var answer pb.Message
				if err := protobuf.NewReader(s).ReadMsg(&answer); err != nil {
					t.Fatal(err)
				}
				if err := protobuf.NewWriter(s).WriteMsg(tc.second); err != nil {
					t.Fatal(err)
				}
			}
			records := p.records(t, rangesync.ReconcileStreamName)
			_ = s.Reset()
			assertDisconnect(t, records[0].Err())
			assertFailed(t, b.scorer, p.addr)
			if p.resets.Load() == 0 {
				t.Fatal("stream not reset")
			}
			if len(b.responderWants(p.addr)) != 0 {
				t.Fatal("wants from a failed session")
			}
		})
	}
}

func TestReconcileInvalidFromResponder(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		answer []byte
	}{
		{"undecodable", []byte{3, 0xff, 0xff, 0xff}},
		{"oversize", binary.AppendUvarint(nil, rangesync.MaxMessageBytes+1)},
		{"busy with parts", mustMarshalDelimited(t, &pb.Message{Busy: true, Parts: msg(fpPart(rangeOf("0"), fingerprint.Summary{})).Parts})},
		{"unsolicited fingerprint", mustMarshalDelimited(t, msg(fpPart(rangeOf("0101"), fingerprint.Summary{Count: 1, Digest: [32]byte{1}})))},
		{"symbols to a Phase-1 node", mustMarshalDelimited(t, msg(symbolsPart(rangeOf(""))))},
		{"more to a Phase-1 node", mustMarshalDelimited(t, msg(morePart(rangeOf(""))))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a := newTestNode(t, stampedChunks(t, 20)...)
			a.start(t)
			// a fake responder answers the opening with tc.answer
			evil := swarm.RandAddress(t)
			a.rec.SetProtocols(p2p.ProtocolSpec{
				Name:    rangesync.ProtocolName,
				Version: rangesync.ProtocolVersion,
				StreamSpecs: []p2p.StreamSpec{{
					Name: rangesync.ReconcileStreamName,
					Handler: func(_ context.Context, _ p2p.Peer, s p2p.Stream) error {
						var open pb.Message
						if err := protobuf.NewReader(s).ReadMsg(&open); err != nil {
							return err
						}
						if _, err := s.Write(tc.answer); err != nil {
							return err
						}
						_, _ = io.Copy(io.Discard, s)
						return nil
					},
				}},
			})
			p := &rawPeer{addr: evil, rec: streamtest.New(streamtest.WithBaseAddr(evil), streamtest.WithProtocols(a.svc.Protocol()))}
			if err := p.sendStatus(t, peerStatus(fingerprint.DefaultMapperID)); err != nil {
				t.Fatal(err)
			}

			wants, err := a.svc.Reconcile(context.Background(), evil)
			if !errors.Is(err, rangesync.ErrInvalidMessage) {
				t.Fatalf("got %v, want %v", err, rangesync.ErrInvalidMessage)
			}
			if wants != nil {
				t.Fatalf("wants: %v", wants)
			}
			if !a.rec.IsDisconnected(evil) {
				t.Fatal("peer not disconnected")
			}
			if blocked, _ := a.rec.IsBlocklisted(evil); blocked {
				t.Fatal("peer blocklisted")
			}
			assertFailed(t, a.scorer, evil)
		})
	}
}

func TestStart(t *testing.T) {
	t.Parallel()

	store := mockstorer.NewReserve() // no address index
	svc := rangesync.New(streamtest.NewRecorderDisconnecter(streamtest.New()), store, swarm.RandAddress(t), nil, nil, nil, log.Noop, rangesync.Options{})
	if err := svc.Start(); !errors.Is(err, storer.ErrAddrIndexDisabled) {
		t.Fatalf("got %v, want %v", err, storer.ErrAddrIndexDisabled)
	}
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestReconcileInitiatorSessionLimit checks that the session caps also bound
// outgoing sessions: Reconcile fails with ErrSessionLimit without a stream.
func TestReconcileInitiatorSessionLimit(t *testing.T) {
	t.Parallel()

	assertLimit := func(t *testing.T, a, b *testNode) {
		t.Helper()
		_, err := a.svc.Reconcile(context.Background(), b.addr)
		if !errors.Is(err, rangesync.ErrSessionLimit) {
			t.Fatalf("got %v, want %v", err, rangesync.ErrSessionLimit)
		}
		if fails, succeeds := a.scorer.get(b.addr); len(fails) != 0 || succeeds != 0 {
			t.Fatalf("scorer: %v fails, %d succeeds", fails, succeeds)
		}
	}

	t.Run("peer session running", func(t *testing.T) {
		t.Parallel()
		a := newTestNode(t, stampedChunks(t, 20)...)
		b := newTestNode(t, stampedChunks(t, 20)...)
		a.start(t)
		b.start(t)
		connect(t, a, b)
		// a raw peer with b's address holds an incoming session on a
		p := &rawPeer{addr: b.addr, rec: streamtest.New(streamtest.WithBaseAddr(b.addr), streamtest.WithProtocols(a.svc.Protocol()))}
		openSession(t, p)
		assertLimit(t, a, b)
	})

	t.Run("node-wide cap", func(t *testing.T) {
		t.Parallel()
		a := newTestNode(t, stampedChunks(t, 20)...)
		b := newTestNode(t, stampedChunks(t, 20)...)
		a.start(t)
		b.start(t)
		connect(t, a, b)
		for range rangesync.MaxConcurrentSessions {
			p := newRawPeer(t, a)
			if err := p.sendStatus(t, peerStatus(fingerprint.DefaultMapperID)); err != nil {
				t.Fatal(err)
			}
			openSession(t, p)
		}
		assertLimit(t, a, b)
	})

	t.Run("slot released after session", func(t *testing.T) {
		t.Parallel()
		a := newTestNode(t, stampedChunks(t, 20)...)
		b := newTestNode(t, stampedChunks(t, 20)...)
		a.start(t)
		b.start(t)
		connect(t, a, b)
		for range 2 {
			if _, err := a.svc.Reconcile(context.Background(), b.addr); err != nil {
				t.Fatal(err)
			}
			// the initiator returns once it has read the last message; the
			// responder's handler releases its slot only after that, so
			// wait for it before the next session (else b answers Busy)
			deadline := time.Now().Add(5 * time.Second)
			for a.svc.SessionsInUse() != 0 || b.svc.SessionsInUse() != 0 {
				if time.Now().After(deadline) {
					t.Fatal("session slots not released")
				}
				runtime.Gosched()
			}
		}
	})
}

func TestReconcileWithStripe(t *testing.T) {
	t.Parallel()

	chs := stampedChunks(t, 3000)
	a := newTestNode(t)
	b := newTestNode(t, chs...)
	a.start(t)
	b.start(t)
	connect(t, a, b)

	st, err := a.svc.LocalStatus()
	if err != nil {
		t.Fatal(err)
	}
	stripe := st.Root.Child(1, 1)
	var want []storer.ChunkID
	for _, ch := range chs {
		if stripe.Contains(ch.Address()) {
			want = append(want, chunkIDOf(t, ch))
		}
	}

	var (
		got   []storer.ChunkID
		calls int
	)
	wants, err := a.svc.ReconcileWith(context.Background(), b.addr, rangesync.ReconcileOptions{
		Open: []fingerprint.Range{stripe},
		Wants: func(ids []storer.ChunkID) {
			calls++
			got = append(got, ids...)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(wants) != 0 {
		t.Fatalf("returned %d wants with a Wants callback", len(wants))
	}
	if calls == 0 {
		t.Fatal("Wants not called")
	}
	// the opening is the stripe only: every want is in it, each listed once
	assertChunkIDs(t, got, want...)
	sent, _ := reconcileMessages(t, a, b.addr)
	fps, _ := partsOf(t, sent[0])
	if len(fps) != 1 || !fps[0].Equal(stripe) {
		t.Fatalf("opening %v, want %s", fps, stripe)
	}
}
