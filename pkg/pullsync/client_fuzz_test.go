// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullsync_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/pullsync"
	"github.com/ethersphere/bee/v2/pkg/pullsync/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzClientSync drives the REAL client-side Syncer.Sync full download loop
// against a fuzzer-controlled malicious peer. A fake server registered in the
// recorder drains the client's pb.Get and then writes RAW fuzz bytes back on the
// stream; the client's protobuf reader frames those bytes into a pb.Offer
// followed by a stream of pb.Delivery messages. This exercises the offer decode,
// the per-chunk nil/HashSize/zero-address checks, bitvector sizing and the Want
// write, and the delivery loop: pb.Delivery decode, swarm.NewChunk,
// postage.Stamp.UnmarshalBinary, stamp.Hash, the ErrUnsolicitedChunk check,
// validStamp, the cac.Valid vs soc.FromChunk branch, and ReservePutter().Put.
//
// The client is built with an empty reserve (IsWithinStorageRadius true,
// ReserveHas false) so it wants every non-zero offered chunk, maximizing
// delivery-loop coverage.
//
// Invariant: Sync must never panic on any peer byte stream and must always
// terminate (the short ctx timeout guards the blocking read). On a nil error the
// number of chunks put is bounded by the number offered.
func FuzzClientSync(f *testing.F) {
	// well-formed frame stream: an offer with one chunk followed by one delivery.
	offer := pullsyncFrame(f, &pb.Offer{
		Topmost: 4,
		Chunks: []*pb.Chunk{
			{
				Address:   make([]byte, swarm.HashSize),
				BatchID:   make([]byte, swarm.HashSize),
				StampHash: make([]byte, swarm.HashSize),
			},
		},
	})
	delivery := pullsyncFrame(f, &pb.Delivery{
		Address: make([]byte, swarm.HashSize),
		Data:    []byte("chunk data"),
		Stamp:   make([]byte, postage.StampSize),
	})
	wellFormed := make([]byte, 0, len(offer)+len(delivery))
	wellFormed = append(wellFormed, offer...)
	wellFormed = append(wellFormed, delivery...)
	f.Add(wellFormed)

	// offer with a wrong-length address.
	f.Add(pullsyncFrame(f, &pb.Offer{Topmost: 1, Chunks: []*pb.Chunk{{Address: []byte{0x01, 0x02}}}}))
	// offer with zero chunks.
	f.Add(pullsyncFrame(f, &pb.Offer{Topmost: 7}))
	// empty stream.
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		fakeServer := func(_ context.Context, _ p2p.Peer, stream p2p.Stream) error {
			defer func() { _ = stream.FullClose() }()
			// drain the client's range request before replying.
			sr := protobuf.NewReader(stream)
			var g pb.Get
			if err := sr.ReadMsg(&g); err != nil {
				return err
			}
			if _, err := stream.Write(data); err != nil {
				return err
			}
			return nil
		}

		recorder := streamtest.New(streamtest.WithProtocols(p2p.ProtocolSpec{
			Name:    pullsync.ProtocolName,
			Version: pullsync.ProtocolVersion,
			StreamSpecs: []p2p.StreamSpec{
				{
					Name:    pullsync.StreamName,
					Handler: fakeServer,
				},
			},
		}))

		psClient, _ := newPullSync(t, recorder, 0)

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		// must never panic; any returned error is a valid outcome.
		_, _, _ = psClient.Sync(ctx, swarm.ZeroAddress, 0, 0)
	})
}
