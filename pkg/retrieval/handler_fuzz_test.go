// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package retrieval_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	pricermock "github.com/ethersphere/bee/v2/pkg/pricer/mock"
	"github.com/ethersphere/bee/v2/pkg/retrieval"
	pb "github.com/ethersphere/bee/v2/pkg/retrieval/pb"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/swarm"

	accountingmock "github.com/ethersphere/bee/v2/pkg/accounting/mock"
	topologymock "github.com/ethersphere/bee/v2/pkg/topology/mock"
)

// FuzzHandler drives arbitrary request bytes through the real retrieval
// Service.handler over the streamtest recorder — the full server-side trust
// boundary a remote peer can reach: the protobuf decode of pb.Request, the
// swarm.NewAddress(req.Addr) construction, the IsZero/IsEmpty/IsValidLength
// validity gate, the storer.Lookup().Get, the ErrNotFound forwarding branch,
// pricer.Price, accounting.PrepareDebit/Apply and the pb.Delivery write-back
// (both the error and success paths).
//
// The server is wired with an empty topology so the forwarding branch fails
// fast with "no peer found" and cannot recurse or hang; a 3s context timeout is
// the backstop. Its store holds exactly one fixture chunk.
//
// The target asserts the handler never panics on hostile input and upholds the
// one property that always holds on success: because forwarding is disabled and
// the store contains only the fixture chunk, a success delivery must carry that
// chunk's data.
func FuzzHandler(f *testing.F) {
	fixture := testingc.FixtureChunk("0033")

	f.Add(fixture.Address().Bytes())
	f.Add(make([]byte, swarm.HashSize))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, addr []byte) {
		serverStorer := &testStorer{ChunkStore: inmemchunkstore.New()}
		if err := serverStorer.Put(context.Background(), fixture); err != nil {
			t.Fatal(err)
		}

		serverAddr := swarm.MustParseHexAddress("0034")
		clientAddr := swarm.MustParseHexAddress("9ee7add8")

		server := createRetrieval(
			t, serverAddr, serverStorer, nil,
			topologymock.NewTopologyDriver(), log.Noop,
			accountingmock.NewAccounting(),
			pricermock.NewMockService(defaultPrice, defaultPrice),
			nil, false,
		)

		recorder := streamtest.New(
			streamtest.WithProtocols(server.Protocol()),
			streamtest.WithBaseAddr(clientAddr),
		)

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		stream, err := recorder.NewStream(ctx, serverAddr, nil, retrieval.ProtocolName, retrieval.ProtocolVersion, retrieval.StreamName)
		if err != nil {
			t.Fatalf("new stream: %v", err)
		}

		w, r := protobuf.NewWriterAndReader(stream)
		if err := w.WriteMsgWithContext(ctx, &pb.Request{Addr: addr}); err != nil {
			// the handler may reset the stream on a rejected request before the
			// write completes; that is a valid outcome, not a defect.
			_ = stream.Close()
			return
		}

		var d pb.Delivery
		if err := r.ReadMsgWithContext(ctx, &d); err != nil {
			_ = stream.Close()
			return
		}

		// A success delivery (no error) can only be the single fixture chunk,
		// since forwarding is disabled and the store holds nothing else.
		if d.Err == "" {
			if !bytes.Equal(d.Data, fixture.Data()) {
				t.Fatalf("success delivery data %x does not match fixture chunk data %x", d.Data, fixture.Data())
			}
		}

		_ = stream.Close()
	})
}

// FuzzClientDelivery drives peer-controlled delivery bytes through the real
// client-side path Service.retrieveChunk, driven from the public RetrieveChunk:
// reading a peer-supplied pb.Delivery, the d.Err != "" ->
// p2p.NewChunkDeliveryError branch, the chunk construction and the
// cac.Valid/soc.Valid validity check that raises swarm.ErrInvalidChunk.
//
// A malicious-peer protocol handler drains the outgoing pb.Request and replies
// with a pb.Delivery carrying the fuzzed Data/Err. Origin retries are bounded:
// on an invalid chunk the peer is skipped, so the next closestPeer lookup finds
// no peer and the loop terminates; a 3s context timeout is the backstop.
//
// The target asserts neither RetrieveChunk nor the delivery path panics on
// hostile input and upholds the property that always holds on success: a
// non-error result returns a non-nil chunk whose address equals the requested
// address and which passes cac.Valid or soc.Valid.
func FuzzClientDelivery(f *testing.F) {
	validCac, err := cac.New([]byte("fuzz retrieval payload"))
	if err != nil {
		f.Fatal(err)
	}
	// requested address matches the valid CAC so the success path is reachable.
	chunkAddr := validCac.Address()

	f.Add(validCac.Data(), "")
	f.Add([]byte("x"), "not found")
	f.Add([]byte{}, "")

	f.Fuzz(func(t *testing.T, data []byte, errStr string) {
		serverAddr := swarm.MustParseHexAddress("0034")
		clientAddr := swarm.MustParseHexAddress("9ee7add8")

		maliciousSpec := p2p.ProtocolSpec{
			Name:    retrieval.ProtocolName,
			Version: retrieval.ProtocolVersion,
			StreamSpecs: []p2p.StreamSpec{
				{
					Name: retrieval.StreamName,
					Handler: func(ctx context.Context, _ p2p.Peer, stream p2p.Stream) error {
						w, r := protobuf.NewWriterAndReader(stream)
						var req pb.Request
						if err := r.ReadMsgWithContext(ctx, &req); err != nil {
							_ = stream.Reset()
							return err
						}
						if err := w.WriteMsgWithContext(ctx, &pb.Delivery{Data: data, Err: errStr}); err != nil {
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
			streamtest.WithBaseAddr(serverAddr),
		)

		client := createRetrieval(
			t, clientAddr, &testStorer{ChunkStore: inmemchunkstore.New()}, recorder,
			topologymock.NewTopologyDriver(topologymock.WithClosestPeer(serverAddr)), log.Noop,
			accountingmock.NewAccounting(),
			pricermock.NewMockService(defaultPrice, defaultPrice),
			nil, false,
		)

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		chunk, err := client.RetrieveChunk(ctx, chunkAddr, swarm.ZeroAddress)
		if err != nil {
			return
		}

		// On the success path the returned chunk must be non-nil, address the
		// requested chunk, and pass the content-address / single-owner check.
		if chunk == nil {
			t.Fatal("success returned a nil chunk")
		}
		if !chunk.Address().Equal(chunkAddr) {
			t.Fatalf("success chunk address %s does not match requested %s", chunk.Address(), chunkAddr)
		}
		if !cac.Valid(chunk) && !soc.Valid(chunk) {
			t.Fatalf("success chunk %s is neither a valid CAC nor SOC", chunk.Address())
		}
	})
}
