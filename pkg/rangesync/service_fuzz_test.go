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
	"testing"

	"github.com/coreos/go-semver/semver"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// inputStream is a p2p.Stream that reads a fixed input, then io.EOF, and
// discards what is written to it.
type inputStream struct {
	io.Reader
	writes int
}

func (s *inputStream) Write(p []byte) (int, error)     { s.writes++; return len(p), nil }
func (*inputStream) Close() error                      { return nil }
func (*inputStream) FullClose() error                  { return nil }
func (*inputStream) Reset() error                      { return nil }
func (*inputStream) Headers() p2p.Headers              { return nil }
func (*inputStream) ResponseHeaders() p2p.Headers      { return nil }
func (*inputStream) Version() (*semver.Version, error) { return nil, nil }

// handler returns the handler of stream name of n's service.
func handler(tb testing.TB, n *testNode, name string) p2p.HandlerFunc {
	tb.Helper()
	for _, s := range n.svc.Protocol().StreamSpecs {
		if s.Name == name {
			return s.Handler
		}
	}
	tb.Fatalf("no stream %q", name)
	return nil
}

// checkHandlerError checks that a handler error that disconnects the peer
// reports a violation.
func checkHandlerError(t *testing.T, err error) {
	t.Helper()
	var de *p2p.DisconnectError
	if errors.As(err, &de) && !errors.Is(err, rangesync.ErrInvalidMessage) {
		t.Fatalf("disconnect without a violation: %v", err)
	}
}

// FuzzHandleStatus feeds the status handler an arbitrary stream. It must not
// panic; a disconnect is a violation; an accepted status is recorded as
// ParseStatus decodes it.
func FuzzHandleStatus(f *testing.F) {
	n := newTestNode(f)
	h := handler(f, n, rangesync.StatusStreamName)

	valid := peerStatus(fingerprint.DefaultMapperID)
	full := rangesync.EncodeStatus(rangesync.Status{
		Root:     fingerprint.NewRange(swarm.NewAddress(bytes.Repeat([]byte{0xff}, swarm.HashSize)), 200),
		Summary:  fingerprint.Summary{Count: 1 << 40, Digest: [32]byte{1}},
		LeafBits: 55,
		Mapping:  1<<32 - 1,
	})
	full.AuditRound = 1<<64 - 1
	full.AuditDigest = bytes.Repeat([]byte{2}, 32)
	f.Add(mustMarshalDelimited(f, valid))
	f.Add(mustMarshalDelimited(f, full))
	f.Add(mustMarshalDelimited(f, peerStatus(2)))
	f.Add([]byte{})
	f.Add([]byte{0})
	f.Add([]byte{3, 0xff, 0xff, 0xff})
	f.Add(binary.AppendUvarint(nil, 1<<20))
	f.Add(mustMarshalDelimited(f, valid)[:5])

	f.Fuzz(func(t *testing.T, data []byte) {
		peer := swarm.RandAddress(t)
		err := h(context.Background(), p2p.Peer{Address: peer}, &inputStream{Reader: bytes.NewReader(data)})
		checkHandlerError(t, err)
		st, ok := n.svc.LastStatus(peer)
		if ok != (err == nil) {
			t.Fatalf("status recorded %v with error %v", ok, err)
		}
		if !ok {
			return
		}
		var m pb.Status
		l, k := binary.Uvarint(data)
		if k <= 0 || m.Unmarshal(data[k:k+int(l)]) != nil {
			t.Fatal("accepted an undecodable status")
		}
		want, err := rangesync.ParseStatus(&m)
		if err != nil {
			t.Fatalf("accepted an invalid status: %v", err)
		}
		if !st.Root.Equal(want.Root) || st.Summary != want.Summary || st.LeafBits != want.LeafBits || st.Mapping != want.Mapping {
			t.Fatalf("recorded %+v, want %+v", st, want)
		}
	})
}

// FuzzHandleReconcile feeds the reconcile handler an arbitrary stream of
// messages from a peer whose status it has. It must not panic or hang; a
// disconnect is a violation; the wants of a completed session are IDs this
// node does not hold.
func FuzzHandleReconcile(f *testing.F) {
	chs := stampedChunks(f, 40)
	n := newTestNode(f, chs...)
	n.start(f)
	peer := swarm.RandAddress(f)
	if err := handler(f, n, rangesync.StatusStreamName)(context.Background(), p2p.Peer{Address: peer}, &inputStream{Reader: bytes.NewReader(mustMarshalDelimited(f, peerStatus(fingerprint.DefaultMapperID)))}); err != nil {
		f.Fatal(err)
	}
	h := handler(f, n, rangesync.ReconcileStreamName)

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	st, err := n.svc.LocalStatus()
	if err != nil {
		f.Fatal(err)
	}
	delimited := func(ms ...*pb.Message) []byte {
		b := make([]byte, 0, 1024)
		for _, m := range ms {
			b = append(b, mustMarshalDelimited(f, m)...)
		}
		return b
	}
	differs := fingerprint.Summary{Count: 1, Digest: [32]byte{1}}
	children := make([]*pb.Part, 0, 1<<fingerprint.FanoutBits)
	for _, c := range root.Children(fingerprint.FanoutBits) {
		children = append(children, fpPart(c, differs))
	}
	other := stampedChunk(f)
	otherID := chunkIDOf(f, other)
	f.Add(delimited(msg(fpPart(root, st.Summary))))
	f.Add(delimited(msg(fpPart(root, differs)), msg(children...), msg()))
	f.Add(delimited(msg(fpPart(rangeOf("0110"), fingerprint.Summary{})), msg()))
	f.Add(delimited(msg(fpPart(rangeOf("1"), differs)), msg(itemsPart(rangesync.Items{
		Range: rangeOf("1"),
		IDs:   []fingerprint.ID{fingerprint.NewID(otherID.Address, otherID.StampHash)},
	}))))
	f.Add(delimited(&pb.Message{Busy: true}))
	f.Add(delimited(msg()))
	f.Add([]byte{})
	f.Add([]byte{3, 0xff, 0xff, 0xff})
	f.Add(binary.AppendUvarint(nil, rangesync.MaxMessageBytes+1))

	f.Fuzz(func(t *testing.T, data []byte) {
		n.mu.Lock()
		clear(n.wants)
		n.mu.Unlock()
		err := h(context.Background(), p2p.Peer{Address: peer}, &inputStream{Reader: bytes.NewReader(data)})
		checkHandlerError(t, err)
		wants := n.responderWants(peer)
		if err != nil && len(wants) > 0 {
			t.Fatal("wants from a failed session")
		}
		for _, w := range wants {
			if has, _ := n.store.ReserveHasID(w.Address, w.StampHash); has {
				t.Fatalf("want %s is held", w.Address)
			}
		}
	})
}
