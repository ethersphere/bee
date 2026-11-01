// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullsync_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/pullsync"
	"github.com/ethersphere/bee/v2/pkg/pullsync/pb"
	mock "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzHandlerWant drives the REAL server-side Syncer.handler end-to-end over the
// streamtest recorder with a peer-controlled Want bitvector. A valid pb.Get is
// written, the deterministic (non-empty) offer built from the package-level
// `results`/`chunks` is read back, and then the fuzzed bitvector is delivered as
// the Want frame. This exercises the protobuf decode of the Want, processWant's
// bitvector.NewFromBytes(want.BitVector, len(offer.Chunks)) sizing, the per-index
// bv.Get loop with ReserveGet lookups, and the rate limiter path.
//
// maxPage == len(results) (5) forces collectAddrs to hit its limit and return
// immediately instead of blocking on the 1s pageTimeout.
//
// Invariants: the handler must never panic on any bitvector, and every non-zero
// address it delivers must be one of the addresses it offered this peer (misses
// become zero-address deliveries; a valid bitvector may legally select none).
func FuzzHandlerWant(f *testing.F) {
	// ceil(5/8) == 1 byte covers all five offered chunks.
	f.Add([]byte{0x00})                   // want nothing
	f.Add([]byte{0x1f})                   // want all five
	f.Add([]byte{0xff})                   // extra bits set
	f.Add([]byte{})                       // empty bitvector
	f.Add([]byte{0xff, 0xff, 0xff, 0xff}) // over-long bitvector

	f.Fuzz(func(t *testing.T, bitvector []byte) {
		ps, _ := newPullSync(t, nil, 5, mock.WithSubscribeResp(results, nil), mock.WithChunks(chunks...))
		recorder := streamtest.New(streamtest.WithProtocols(ps.Protocol()))

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		stream, err := recorder.NewStream(ctx, swarm.ZeroAddress, nil, pullsync.ProtocolName, pullsync.ProtocolVersion, pullsync.StreamName)
		if err != nil {
			t.Fatalf("new stream: %v", err)
		}

		w, r := protobuf.NewWriterAndReader(stream)

		if err := w.WriteMsgWithContext(ctx, &pb.Get{Bin: 0, Start: 0}); err != nil {
			_ = stream.Close()
			return
		}

		var offer pb.Offer
		if err := r.ReadMsgWithContext(ctx, &offer); err != nil {
			_ = stream.Close()
			return
		}

		offered := make(map[string]struct{}, len(offer.Chunks))
		for _, c := range offer.Chunks {
			if c == nil {
				continue
			}
			offered[string(c.Address)] = struct{}{}
		}

		if err := w.WriteMsgWithContext(ctx, &pb.Want{BitVector: bitvector}); err != nil {
			_ = stream.Close()
			return
		}

		for {
			var d pb.Delivery
			if err := r.ReadMsgWithContext(ctx, &d); err != nil {
				break
			}
			addr := swarm.NewAddress(d.Address)
			if addr.Equal(swarm.ZeroAddress) {
				// a store miss is delivered as a zero-address chunk
				continue
			}
			if _, ok := offered[string(d.Address)]; !ok {
				t.Fatalf("handler delivered address %x that was not in the offer", d.Address)
			}
		}

		_ = stream.Close()
	})
}
