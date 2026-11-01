// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/pushsync"
	"github.com/ethersphere/bee/v2/pkg/pushsync/pb"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/topology/mock"
)

// fuzzSigner is generated once and reused across executions so the fuzzer is not
// dominated by key generation on the receipt-signing path.
var fuzzSigner = func() crypto.Signer {
	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		panic(err)
	}
	return crypto.NewDefaultSigner(key)
}()

var (
	fuzzHandlerSelf = swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")
	fuzzHandlerPeer = swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")
)

// FuzzHandlerDelivery drives arbitrary chunk deliveries through the real
// PushSync.handler over the streamtest recorder — the full protocol read path a
// remote peer can reach: protobuf decode, chunk construction, the CAC/SOC
// branch, stamp decoding, and (for accepted chunks) the store-and-sign receipt
// path. The receiving node is wired with ErrWantSelf so accepted chunks are
// stored locally rather than forwarded.
//
// The target asserts the handler never panics on hostile input and upholds the
// receipt integrity property: a success receipt acknowledges exactly the address
// that was delivered.
func FuzzHandlerDelivery(f *testing.F) {
	// valid content-addressed chunk delivery
	cacChunk, err := cac.New([]byte("fuzz push payload"))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(cacChunk.Address().Bytes(), cacChunk.Data(), make([]byte, postage.StampSize))

	// valid single-owner chunk delivery
	socChunk, err := soc.New(make([]byte, swarm.HashSize), cacChunk).Sign(fuzzSigner)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(socChunk.Address().Bytes(), socChunk.Data(), make([]byte, postage.StampSize))

	// degenerate inputs
	f.Add(make([]byte, swarm.HashSize), []byte("not a real chunk"), []byte{})
	f.Add([]byte{}, []byte{}, []byte{})

	f.Fuzz(func(t *testing.T, addr, data, stamp []byte) {
		ps, _ := createPushSyncNodeWithRadius(
			t, fuzzHandlerSelf, defaultPrices, nil, nil, fuzzSigner, 0, 0,
			mock.WithClosestPeerErr(topology.ErrWantSelf),
		)

		recorder := streamtest.New(
			streamtest.WithProtocols(ps.Protocol()),
			streamtest.WithBaseAddr(fuzzHandlerPeer),
		)

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		stream, err := recorder.NewStream(ctx, fuzzHandlerSelf, nil, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName)
		if err != nil {
			t.Fatalf("new stream: %v", err)
		}

		w, r := protobuf.NewWriterAndReader(stream)
		if err := w.WriteMsgWithContext(ctx, &pb.Delivery{Address: addr, Data: data, Stamp: stamp}); err != nil {
			// the handler may reset the stream on a rejected delivery before the
			// write completes; that is a valid outcome, not a defect.
			_ = stream.Close()
			return
		}

		var rec pb.Receipt
		if err := r.ReadMsgWithContext(ctx, &rec); err != nil {
			_ = stream.Close()
			return
		}

		// A success receipt must acknowledge exactly the delivered address.
		if rec.Err == "" && len(rec.Address) > 0 {
			if !bytes.Equal(rec.Address, swarm.NewAddress(addr).Bytes()) {
				t.Fatalf("success receipt address %x does not match delivered address %x", rec.Address, addr)
			}
		}

		_ = stream.Close()
	})
}
