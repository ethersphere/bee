// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/rangesync/wantset"
	chunktesting "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// validChunks returns n valid content addressed chunks with stamps.
func validChunks(n int) []swarm.Chunk {
	return chunktesting.GenerateTestRandomChunks(n)
}

func idsOf(tb testing.TB, chs ...swarm.Chunk) []storer.ChunkID {
	tb.Helper()
	ids := make([]storer.ChunkID, len(chs))
	for i, ch := range chs {
		ids[i] = chunkIDOf(tb, ch)
	}
	return ids
}

func fpID(c storer.ChunkID) fingerprint.ID {
	return fingerprint.NewID(c.Address, c.StampHash)
}

// sentWants returns the Wants that a sent on its fetch streams to b.
func sentWants(t *testing.T, a *testNode, b swarm.Address) []*pb.Want {
	t.Helper()
	records, err := a.rec.Records(b, rangesync.ProtocolName, rangesync.ProtocolVersion, rangesync.FetchStreamName)
	if err != nil {
		t.Fatal(err)
	}
	var out []*pb.Want
	for _, r := range records {
		ms, err := protobuf.ReadMessages(bytes.NewReader(r.In()), func() protobuf.Message { return new(pb.Want) })
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range ms {
			out = append(out, m.(*pb.Want))
		}
	}
	return out
}

func assertStored(t *testing.T, n *testNode, want bool, ids ...storer.ChunkID) {
	t.Helper()
	for _, id := range ids {
		has, err := n.store.ReserveHasID(id.Address, id.StampHash)
		if err != nil {
			t.Fatal(err)
		}
		if has != want {
			t.Fatalf("chunk %s stored %v, want %v", id.Address, has, want)
		}
	}
}

// assertNoClaims checks that the want set of n holds no claim, and that the
// IDs can be claimed again.
func assertNoClaims(t *testing.T, n *testNode, ids ...storer.ChunkID) {
	t.Helper()
	if l := n.svc.WantSet().Len(); l != 0 {
		t.Fatalf("want set holds %d claims", l)
	}
	other := swarm.RandAddress(t)
	for _, id := range ids {
		if r := n.svc.WantSet().Claim(fpID(id), other); r != wantset.Granted {
			t.Fatalf("claim %s: %v", id.Address, r)
		}
		n.svc.WantSet().Release(fpID(id), other)
	}
}

func assertNotScored(t *testing.T, n *testNode, peer swarm.Address) {
	t.Helper()
	if fails, _ := n.scorer.get(peer); len(fails) != 0 {
		t.Fatalf("scorer fails: %v", fails)
	}
	if n.rec.IsDisconnected(peer) {
		t.Fatal("peer disconnected")
	}
}

func TestFetch(t *testing.T) {
	t.Parallel()

	chs := validChunks(3)
	a := newTestNode(t)
	b := newTestNode(t, chs...)
	connect0(a, b)

	ids := idsOf(t, chs...)
	stored, err := a.svc.Fetch(context.Background(), b.addr, ids)
	if err != nil {
		t.Fatal(err)
	}
	if stored != len(chs) {
		t.Fatalf("stored %d, want %d", stored, len(chs))
	}
	assertStored(t, a, true, ids...)
	assertNoClaims(t, a, ids...)
	assertNotScored(t, a, b.addr)
	assertNotScored(t, b, a.addr)
}

func TestFetchManyWants(t *testing.T) {
	t.Parallel()

	chs := stampedChunks(t, rangesync.MaxWantIDs+10)
	// b holds one chunk of each Want: streamtest deadlocks when a writer
	// runs more than 16 writes ahead of its reader, so the test keeps the
	// deliveries few. b sends no Status, so its shortfall is not judged
	// (shortfall.go; a peer that lists IDs has a Status).
	held := []swarm.Chunk{validChunks(1)[0], validChunks(1)[0]}
	chs[0], chs[len(chs)-1] = held[0], held[1]
	a := newTestNode(t)
	b := newTestNode(t, held...)
	connect0(a, b)

	ids := idsOf(t, chs...)
	stored, err := a.svc.Fetch(context.Background(), b.addr, ids)
	if err != nil {
		t.Fatal(err)
	}
	if stored != len(held) {
		t.Fatalf("stored %d, want %d", stored, len(held))
	}
	wants := sentWants(t, a, b.addr)
	if len(wants) != 2 || len(wants[0].IDs) != rangesync.MaxWantIDs || len(wants[1].IDs) != 10 {
		t.Fatalf("got %d wants", len(wants))
	}
	assertNoClaims(t, a)
	assertNotScored(t, a, b.addr)
}

func TestFetchUnknownID(t *testing.T) {
	t.Parallel()

	chs := validChunks(2)
	a := newTestNode(t)
	b := newTestNode(t, chs[0])
	connect0(a, b)

	ids := idsOf(t, chs...)
	stored, err := a.svc.Fetch(context.Background(), b.addr, ids)
	if err != nil {
		t.Fatal(err)
	}
	if stored != 1 {
		t.Fatalf("stored %d, want 1", stored)
	}
	assertStored(t, a, true, ids[0])
	assertStored(t, a, false, ids[1])
	assertNoClaims(t, a, ids...)
	assertNotScored(t, a, b.addr)
}

func TestFetchSkipsHeld(t *testing.T) {
	t.Parallel()

	chs := validChunks(2)
	a := newTestNode(t, chs[0])
	b := newTestNode(t, chs...)
	connect0(a, b)

	ids := idsOf(t, chs...)
	stored, err := a.svc.Fetch(context.Background(), b.addr, ids)
	if err != nil {
		t.Fatal(err)
	}
	if stored != 1 {
		t.Fatalf("stored %d, want 1", stored)
	}
	wants := sentWants(t, a, b.addr)
	if len(wants) != 1 || len(wants[0].IDs) != 1 || !bytes.Equal(wants[0].IDs[0].Address, ids[1].Address.Bytes()) {
		t.Fatalf("wants %v", wants)
	}
}

func TestFetchSkipsOutOfRadius(t *testing.T) {
	t.Parallel()

	chs := validChunks(2)
	out := chs[0].Address()
	a := newTestNodeOpts(t, nodeOpts{outside: func(addr swarm.Address) bool { return addr.Equal(out) }})
	b := newTestNode(t, chs...)
	connect0(a, b)

	ids := idsOf(t, chs...)
	stored, err := a.svc.Fetch(context.Background(), b.addr, ids)
	if err != nil {
		t.Fatal(err)
	}
	if stored != 1 {
		t.Fatalf("stored %d, want 1", stored)
	}
	wants := sentWants(t, a, b.addr)
	if len(wants) != 1 || len(wants[0].IDs) != 1 || !bytes.Equal(wants[0].IDs[0].Address, ids[1].Address.Bytes()) {
		t.Fatalf("wants %v", wants)
	}
	assertStored(t, a, false, ids[0])

	// nothing left to request: no stream is opened
	stored, err = a.svc.Fetch(context.Background(), b.addr, ids[:1])
	if err != nil || stored != 0 {
		t.Fatalf("stored %d, err %v", stored, err)
	}
	if len(sentWants(t, a, b.addr)) != 1 {
		t.Fatal("want sent for an out-of-radius ID")
	}
}

func TestFetchSkipsClaimed(t *testing.T) {
	t.Parallel()

	chs := validChunks(2)
	a := newTestNode(t)
	b := newTestNode(t, chs...)
	connect0(a, b)

	ids := idsOf(t, chs...)
	other := swarm.RandAddress(t)
	if r := a.svc.WantSet().Claim(fpID(ids[0]), other); r != wantset.Granted {
		t.Fatal(r)
	}
	stored, err := a.svc.Fetch(context.Background(), b.addr, ids)
	if err != nil {
		t.Fatal(err)
	}
	if stored != 1 {
		t.Fatalf("stored %d, want 1", stored)
	}
	if l := a.svc.WantSet().Len(); l != 1 {
		t.Fatalf("want set holds %d claims, want 1", l)
	}
}

func TestFetchInvalidStamp(t *testing.T) {
	t.Parallel()

	chs := validChunks(2)
	bad := chs[0].Address()
	a := newTestNodeOpts(t, nodeOpts{validStamp: func(ch swarm.Chunk) (swarm.Chunk, error) {
		if ch.Address().Equal(bad) {
			return nil, errors.New("invalid stamp")
		}
		return ch, nil
	}})
	b := newTestNode(t, chs...)
	connect0(a, b)

	ids := idsOf(t, chs...)
	stored, err := a.svc.Fetch(context.Background(), b.addr, ids)
	if err != nil {
		t.Fatal(err)
	}
	if stored != 1 {
		t.Fatalf("stored %d, want 1", stored)
	}
	assertStored(t, a, false, ids[0])
	assertStored(t, a, true, ids[1])
	assertNoClaims(t, a, ids...)
	assertNotScored(t, a, b.addr)
}

// fakeHolder replaces b's fetch handler with deliver, which gets the IDs of
// the Want and writes Deliveries.
func fakeHolder(a, b *testNode, deliver func(want *pb.Want, w protobuf.Writer)) {
	spec := b.svc.Protocol()
	for i := range spec.StreamSpecs {
		if spec.StreamSpecs[i].Name != rangesync.FetchStreamName {
			continue
		}
		spec.StreamSpecs[i].Handler = func(ctx context.Context, _ p2p.Peer, s p2p.Stream) error {
			w, r := protobuf.NewWriterAndReader(s)
			var want pb.Want
			if err := r.ReadMsgWithContext(ctx, &want); err != nil {
				return err
			}
			deliver(&want, w)
			return s.FullClose()
		}
	}
	a.rec.SetProtocols(spec)
}

func delivery(t *testing.T, ch swarm.Chunk) *pb.Delivery {
	t.Helper()
	st, err := ch.Stamp().MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	return &pb.Delivery{Address: ch.Address().Bytes(), Data: ch.Data(), Stamp: st}
}

// TestFetchStreamsPerPeer checks the requester side's cap on its concurrent
// fetch streams to one peer: fetches from every source (round, stripe,
// retry, served sessions' wants) run through FetchWithResult, and however
// many run at once, at most MaxFetchStreamsOut streams to the peer are open,
// below the peer's own limit (MaxPeerFetchStreams), whose handler would
// disconnect this node past it. Then every fetch completes, and none of
// them hits the peer's limit.
func TestFetchStreamsPerPeer(t *testing.T) {
	t.Parallel()

	if rangesync.MaxFetchStreamsOut >= rangesync.MaxPeerFetchStreams {
		t.Fatalf("requester cap %d not below the peer's limit %d", rangesync.MaxFetchStreamsOut, rangesync.MaxPeerFetchStreams)
	}
	const fetches = 2 * rangesync.MaxPeerFetchStreams
	chs := validChunks(fetches + 1)
	a := newTestNode(t)
	b := newTestNode(t, chs...)

	// b's own fetch handler, held at a gate that counts the streams
	var inFlight, peak atomic.Int32
	gate := make(chan struct{})
	spec := b.svc.Protocol()
	for i := range spec.StreamSpecs {
		if spec.StreamSpecs[i].Name != rangesync.FetchStreamName {
			continue
		}
		handle := spec.StreamSpecs[i].Handler
		spec.StreamSpecs[i].Handler = func(ctx context.Context, p p2p.Peer, s p2p.Stream) error {
			n := inFlight.Add(1)
			for m := peak.Load(); n > m && !peak.CompareAndSwap(m, n); m = peak.Load() {
			}
			<-gate
			defer inFlight.Add(-1)
			return handle(ctx, p, s)
		}
	}
	a.rec.SetProtocols(spec)
	b.rec.SetProtocols(a.svc.Protocol())

	ids := idsOf(t, chs[:fetches]...)
	errs := make(chan error, fetches)
	for _, id := range ids {
		go func() {
			_, err := a.svc.Fetch(context.Background(), b.addr, []storer.ChunkID{id})
			errs <- err
		}()
	}
	waitFor(t, "streams at the cap", func() bool { return inFlight.Load() >= rangesync.MaxFetchStreamsOut })
	time.Sleep(50 * time.Millisecond) // more streams, if uncapped, arrive meanwhile
	if got := peak.Load(); got > rangesync.MaxFetchStreamsOut {
		t.Fatalf("%d fetch streams open to the peer at once, want at most %d", got, rangesync.MaxFetchStreamsOut)
	}
	// the fetches that wait for a stream hold no claims
	if l := a.svc.WantSet().Len(); l != rangesync.MaxFetchStreamsOut {
		t.Fatalf("want set holds %d claims, want %d (the open streams' only)", l, rangesync.MaxFetchStreamsOut)
	}
	// a fetch whose context ends while it waits returns, having claimed nothing
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	extra := idsOf(t, chs[fetches])
	if _, err := a.svc.FetchWithResult(ctx, b.addr, extra); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("fetch waiting for a stream: got %v, want %v", err, context.DeadlineExceeded)
	}
	if a.svc.WantSet().HeldBy(fpID(extra[0]), b.addr) {
		t.Fatal("a fetch that waited for a stream left its claim")
	}
	close(gate)
	for range fetches {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	assertStored(t, a, true, ids...)
	records, err := a.rec.Records(b.addr, rangesync.ProtocolName, rangesync.ProtocolVersion, rangesync.FetchStreamName)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range records {
		if err := r.Err(); err != nil {
			t.Fatalf("peer's fetch handler: %v", err)
		}
	}
	assertNotScored(t, a, b.addr)
}

// TestFetchStreamsLaggingHandlers checks the margin of the requester's cap
// below the peer's limit: fetch streams this node ended (reset) leave their
// handlers on the peer until their next delivery fails, and with two such
// handlers new fetches, more than the cap, still keep the peer within its
// limit (MaxPeerFetchStreams), so the peer does not disconnect this node.
func TestFetchStreamsLaggingHandlers(t *testing.T) {
	t.Parallel()

	const (
		lagging = 2 // the margin the cap leaves
		fresh   = rangesync.MaxFetchStreamsOut + 1
	)
	var calls atomic.Int32
	lag, hold := make(chan struct{}), make(chan struct{})
	chs := validChunks(lagging + fresh)
	a := newTestNode(t)
	b := newTestNodeOpts(t, nodeOpts{getHook: func() {
		if calls.Add(1) <= lagging {
			<-lag
		} else {
			<-hold
		}
	}}, chs...)
	connect0(a, b)

	// fetches whose handlers block on b, ended by this node
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for _, id := range idsOf(t, chs[:lagging]...) {
		wg.Go(func() { _, _ = a.svc.FetchWithResult(ctx, b.addr, []storer.ChunkID{id}) })
	}
	waitFor(t, "handlers on the peer", func() bool { return calls.Load() == lagging })
	cancel()
	wg.Wait()

	// their handlers stay; new fetches
	errs := make(chan error, fresh)
	for _, id := range idsOf(t, chs[lagging:]...) {
		go func() {
			_, err := a.svc.Fetch(context.Background(), b.addr, []storer.ChunkID{id})
			errs <- err
		}()
	}
	waitFor(t, "new handlers", func() bool { return calls.Load() >= rangesync.MaxPeerFetchStreams })
	time.Sleep(50 * time.Millisecond) // a stream past the cap, if any, arrives meanwhile
	close(hold)
	close(lag)
	for range fresh {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	records, err := a.rec.Records(b.addr, rangesync.ProtocolName, rangesync.ProtocolVersion, rangesync.FetchStreamName)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range records {
		if err := r.Err(); errors.Is(err, rangesync.ErrStreamLimitExceeded) {
			t.Fatalf("the peer's fetch stream limit was hit: %v", err)
		}
	}
	assertStored(t, a, true, idsOf(t, chs[lagging:]...)...)
}

func TestFetchViolations(t *testing.T) {
	t.Parallel()

	chs := validChunks(2)
	d0, d1 := delivery(t, chs[0]), delivery(t, chs[1])
	clone := func(d *pb.Delivery) *pb.Delivery {
		return &pb.Delivery{Address: slices.Clone(d.Address), Data: slices.Clone(d.Data), Stamp: slices.Clone(d.Stamp)}
	}
	for _, tc := range []struct {
		name    string
		deliver func() *pb.Delivery
	}{
		{"unsolicited chunk", func() *pb.Delivery { return clone(d1) }},
		{"invalid chunk", func() *pb.Delivery {
			d := clone(d0)
			d.Data[swarm.SpanSize] ^= 1
			return d
		}},
		{"undecodable stamp", func() *pb.Delivery {
			d := clone(d0)
			d.Stamp = d.Stamp[:10]
			return d
		}},
		{"zero address", func() *pb.Delivery {
			d := clone(d0)
			d.Address = swarm.ZeroAddress.Bytes()
			return d
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := newTestNode(t)
			b := newTestNode(t)
			fakeHolder(a, b, func(_ *pb.Want, w protobuf.Writer) {
				_ = w.WriteMsg(tc.deliver())
			})
			ids := idsOf(t, chs[0])
			_, err := a.svc.Fetch(context.Background(), b.addr, ids)
			if !errors.Is(err, rangesync.ErrInvalidMessage) {
				t.Fatalf("got %v, want %v", err, rangesync.ErrInvalidMessage)
			}
			if !a.rec.IsDisconnected(b.addr) {
				t.Fatal("peer not disconnected")
			}
			assertFailed(t, a.scorer, b.addr)
			assertStored(t, a, false, idsOf(t, chs...)...)
			assertNoClaims(t, a, ids...)
		})
	}
}

func TestFetchCrossProtocolDuplicate(t *testing.T) {
	t.Parallel()

	chs := validChunks(1)
	a := newTestNode(t)
	b := newTestNode(t)
	fakeHolder(a, b, func(_ *pb.Want, w protobuf.Writer) {
		// pullsync stores the chunk while the fetch is running
		if err := a.store.ReservePutter().Put(context.Background(), chs[0]); err != nil {
			t.Error(err)
		}
		_ = w.WriteMsg(delivery(t, chs[0]))
	})
	ids := idsOf(t, chs...)
	stored, err := a.svc.Fetch(context.Background(), b.addr, ids)
	if err != nil {
		t.Fatal(err)
	}
	if stored != 0 {
		t.Fatalf("stored %d, want 0", stored)
	}
	if d := a.svc.CrossProtocolDuplicates(); d != 1 {
		t.Fatalf("cross protocol duplicates %v, want 1", d)
	}
	assertNoClaims(t, a, ids...)
}

func TestFetchHandlerInvalidWant(t *testing.T) {
	t.Parallel()

	id := func(addr, sh int) *pb.ID {
		return &pb.ID{Address: make([]byte, addr), StampHash: make([]byte, sh)}
	}
	ids := func(n int) []*pb.ID {
		out := make([]*pb.ID, n)
		for i := range out {
			out[i] = &pb.ID{Address: swarm.RandAddress(t).Bytes(), StampHash: swarm.RandAddress(t).Bytes()}
		}
		return out
	}
	marshal := func(w *pb.Want) []byte {
		var buf bytes.Buffer
		if err := protobuf.NewWriter(&buf).WriteMsg(w); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	dup := ids(2)
	dup[1] = dup[0]

	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"empty", marshal(&pb.Want{})},
		{"oversized", marshal(&pb.Want{IDs: ids(rangesync.MaxWantIDs + 1)})},
		{"short address", marshal(&pb.Want{IDs: []*pb.ID{id(31, 32)}})},
		{"long stamp hash", marshal(&pb.Want{IDs: []*pb.ID{id(32, 33)}})},
		{"empty id", marshal(&pb.Want{IDs: []*pb.ID{{}}})},
		{"duplicate ids", marshal(&pb.Want{IDs: dup})},
		{"undecodable", []byte{3, 0xff, 0xff, 0xff}},
		{"over the size limit", append([]byte{0xff, 0xff, 0x04}, make([]byte, 16)...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := newTestNode(t, validChunks(1)...)
			p := newRawPeer(t, b)
			s := p.open(t, rangesync.FetchStreamName)
			if _, err := s.Write(tc.data); err != nil {
				t.Fatal(err)
			}
			_ = s.Close()
			records := p.records(t, rangesync.FetchStreamName)
			assertDisconnect(t, records[0].Err())
			assertFailed(t, b.scorer, p.addr)
			if p.resets.Load() != 1 {
				t.Fatalf("resets %d, want 1", p.resets.Load())
			}
		})
	}
}

func TestFetchHandlerStreamLimit(t *testing.T) {
	t.Parallel()

	b := newTestNode(t)
	p := newRawPeer(t, b)
	streams := make([]p2p.Stream, rangesync.MaxPeerFetchStreams+1)
	ended := make(chan struct{}, len(streams))
	for i := range streams {
		streams[i] = p.open(t, rangesync.FetchStreamName)
		go func() {
			_, _ = io.ReadAll(streams[i])
			ended <- struct{}{}
		}()
	}
	// the streams within the limit wait in the handler for their Want; the
	// one over it is reset
	select {
	case <-ended:
	case <-time.After(10 * time.Second):
		t.Fatal("no stream reset")
	}
	for _, s := range streams {
		_ = s.Reset()
	}
	for range len(streams) - 1 {
		<-ended
	}
	disconnects := 0
	for _, r := range p.records(t, rangesync.FetchStreamName) {
		var de *p2p.DisconnectError
		if errors.As(r.Err(), &de) && errors.Is(r.Err(), rangesync.ErrStreamLimitExceeded) {
			disconnects++
		}
	}
	if disconnects != 1 {
		t.Fatalf("got %d disconnects, want 1", disconnects)
	}
}

func TestFetchStalledHolder(t *testing.T) {
	t.Parallel()

	chs := validChunks(1)
	a := newTestNodeOpts(t, nodeOpts{fetchTimeout: 100 * time.Millisecond})
	b := newTestNode(t)
	stall := make(chan struct{})
	t.Cleanup(func() { close(stall) })
	fakeHolder(a, b, func(*pb.Want, protobuf.Writer) { <-stall })

	ids := idsOf(t, chs...)
	_, err := a.svc.Fetch(context.Background(), b.addr, ids)
	if !errors.Is(err, rangesync.ErrInvalidMessage) {
		t.Fatalf("got %v, want %v", err, rangesync.ErrInvalidMessage)
	}
	if !a.rec.IsDisconnected(b.addr) {
		t.Fatal("peer not disconnected")
	}
	assertFailed(t, a.scorer, b.addr)
	assertNoClaims(t, a, ids...)
}

func TestFetchClose(t *testing.T) {
	t.Parallel()

	chs := validChunks(1)
	a := newTestNode(t)
	b := newTestNode(t)
	stall := make(chan struct{})
	t.Cleanup(func() { close(stall) })
	wanted := make(chan struct{})
	fakeHolder(a, b, func(*pb.Want, protobuf.Writer) {
		close(wanted)
		<-stall
	})

	ids := idsOf(t, chs...)
	done := make(chan error, 1)
	go func() {
		_, err := a.svc.Fetch(context.Background(), b.addr, ids)
		done <- err
	}()
	<-wanted
	if err := a.svc.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want %v", err, context.Canceled)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("fetch not ended by Close")
	}
	assertNotScored(t, a, b.addr)
}
