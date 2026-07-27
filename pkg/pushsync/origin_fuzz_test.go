// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/pushsync"
	"github.com/ethersphere/bee/v2/pkg/pushsync/pb"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology/mock"
)

// FuzzOriginReceiptLoop drives peer-controlled receipt bytes through the entire
// originator/client push loop of the real PushSync service:
//
//	PushChunkToClosest -> pushToClosest(origin=true) -> push -> pushChunkToPeer
//
// A malicious-peer protocol handler drains the outgoing pb.Delivery and then
// writes the fuzzed bytes back verbatim with stream.Write (bypassing the
// length-delimited protobuf writer) so arbitrary framing reaches the
// originator's ReadMsgWithContext. This exercises strictly more code than the
// decode-only (FuzzReceiptRead) and checkReceipt-only (FuzzCheckReceipt)
// targets: the wire read inside pushChunkToPeer, the rec.Err !=
// "" -> p2p.NewChunkDeliveryError branch, the chunk-address equality guard, the
// origin-branch result switch, the real checkReceipt (signature recovery,
// overlay derivation, proximity and shallow-receipt logic), the ErrShallowReceipt
// path, and the PushChunkToClosest receipt wrapping.
//
// The target asserts the loop never panics on hostile input, and upholds the one
// integrity property that always holds on success: a non-error receipt echoes
// exactly the pushed chunk's address (enforced by the equality guard in
// pushChunkToPeer). Shallow-receipt and error outcomes are legitimate and are
// not asserted against.
func FuzzOriginReceiptLoop(f *testing.F) {
	chunk := testingc.FixtureChunk("7000")

	// A well-formed receipt: address matches the pushed chunk, signature is
	// recoverable over that address, 32-byte nonce, StorageRadius 0. With the
	// originator's radius=0 and tolerance=0 this receipt is never classified as
	// shallow, so it drives the nil-error success branch and the address
	// invariant.
	validSig, err := fuzzSigner.Sign(chunk.Address().Bytes())
	if err != nil {
		f.Fatal(err)
	}
	f.Add(pushsyncFrame(f, &pb.Receipt{
		Address:       chunk.Address().Bytes(),
		Signature:     validSig,
		Nonce:         make([]byte, swarm.HashSize),
		StorageRadius: 0,
	}))

	// Matching address but junk signature: exercises the checkReceipt recover
	// failure path.
	f.Add(pushsyncFrame(f, &pb.Receipt{
		Address:   chunk.Address().Bytes(),
		Signature: make([]byte, 65),
		Nonce:     make([]byte, swarm.HashSize),
	}))

	// Err set: exercises the p2p.NewChunkDeliveryError branch.
	f.Add(pushsyncFrame(f, &pb.Receipt{Err: "rejected"}))

	// Address that does not match the pushed chunk: exercises the equality guard.
	f.Add(pushsyncFrame(f, &pb.Receipt{
		Address:   make([]byte, swarm.HashSize),
		Signature: validSig,
		Nonce:     make([]byte, swarm.HashSize),
	}))

	f.Add([]byte{})
	f.Add([]byte("not length-delimited protobuf"))

	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")
	closestPeer := swarm.MustParseHexAddress("1000000000000000000000000000000000000000000000000000000000000000")

	f.Fuzz(func(t *testing.T, data []byte) {
		// malicious peer: drain the delivery, then reply with raw fuzzed bytes.
		maliciousSpec := p2p.ProtocolSpec{
			Name:    pushsync.ProtocolName,
			Version: pushsync.ProtocolVersion,
			StreamSpecs: []p2p.StreamSpec{
				{
					Name: pushsync.StreamName,
					Handler: func(ctx context.Context, _ p2p.Peer, stream p2p.Stream) error {
						_, r := protobuf.NewWriterAndReader(stream)
						var d pb.Delivery
						if err := r.ReadMsgWithContext(ctx, &d); err != nil {
							_ = stream.Reset()
							return err
						}
						// write the fuzzed bytes verbatim (arbitrary framing)
						if _, err := stream.Write(data); err != nil {
							_ = stream.Reset()
							return err
						}
						return stream.FullClose()
					},
				},
			},
		}

		recorder := streamtest.New(
			streamtest.WithProtocols(maliciousSpec),
			streamtest.WithBaseAddr(closestPeer),
		)

		ps, _ := createPushSyncNodeWithRadius(
			t, pivotNode, defaultPrices, recorder, nil, fuzzSigner, 0, 0,
			mock.WithClosestPeer(closestPeer),
		)

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		receipt, err := ps.PushChunkToClosest(ctx, chunk)

		// On the clean success path the receipt must acknowledge exactly the
		// pushed chunk's address. Shallow-receipt (err != nil) and any other
		// error are legitimate outcomes and must not be asserted against.
		if err == nil && receipt != nil {
			if !receipt.Address.Equal(chunk.Address()) {
				t.Fatalf("success receipt address %s does not match pushed chunk %s", receipt.Address, chunk.Address())
			}
		}
	})
}
