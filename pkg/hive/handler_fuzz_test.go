// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ab "github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/hive"
	"github.com/ethersphere/bee/v2/pkg/hive/pb"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	chequebookmock "github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook/mock"
	"github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	ma "github.com/multiformats/go-multiaddr"
)

// fuzzSignedChequebookRecord builds a genuinely valid, freshly-signed
// pb.BzzAddress carrying a non-zero chequebook. It mirrors
// signedPeerWithChequebook but accepts testing.TB so it can be used from a
// fuzz seed corpus (which only has *testing.F).
func fuzzSignedChequebookRecord(tb testing.TB, networkID uint64, ts int64, cb common.Address) *pb.BzzAddress {
	tb.Helper()

	pk, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		tb.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(pk)
	n := common.HexToHash("0xab").Bytes()

	overlay, err := crypto.NewOverlayAddress(pk.PublicKey, networkID, n)
	if err != nil {
		tb.Fatal(err)
	}

	u, err := ma.NewMultiaddr("/ip4/10.0.0.1/tcp/30303")
	if err != nil {
		tb.Fatal(err)
	}

	addr, err := bzz.NewAddress(signer, []ma.Multiaddr{u}, overlay, networkID, n, ts, cb)
	if err != nil {
		tb.Fatal(err)
	}

	underlayBytes, err := bzz.SerializeUnderlays(addr.Underlays)
	if err != nil {
		tb.Fatal(err)
	}

	return &pb.BzzAddress{
		Underlay:          underlayBytes,
		Overlay:           addr.Overlay.Bytes(),
		Signature:         addr.Signature,
		Nonce:             addr.Nonce,
		Timestamp:         addr.Timestamp,
		ChequebookAddress: addr.ChequebookAddress.Bytes(),
	}
}

// FuzzPeersHandler drives the real Service.peersHandler read path end to end
// over streamtest, on arbitrary peer-supplied bytes:
//
//	ReadMsgWithContext framing decode of pb.Peers -> inLimiter.Allow rate check
//	-> async stream.FullClose -> unbuffered peersChan handoff to the live
//	dispatcher (BootnodeMode=false) -> startCheckPeersHandler -> checkAndAddPeers
//	-> bzz.DeserializeUnderlays / bzz.ParseAddress / bzz.CheckTimestamp /
//	addressbook.Put.
//
// This exercises strictly more real code than the decode-only FuzzPeersRead,
// which bypasses the stream, reader, rate limiter, channel handoff and the
// async goroutine. The fuzzed bytes are written RAW with stream.Write (not the
// length-delimited writer), so the handler's own framing decode is fuzzed
// (malicious-peer pattern). No verifier/storer is configured, so the permissive
// addressbook Put branch is the one that runs.
//
// Synchronization: testutil.CleanupCloser(t, svc) is registered per iteration
// (t is per-iteration), so svc.Close() closes quit and wg.Wait()s the spawned
// checkAndAddPeers goroutine before the iteration ends, surfacing any async
// panic deterministically without asserting on side effects.
//
// Invariant: must not panic. The handler resetting the stream or returning an
// error, and any (or zero) addressbook additions, are all valid outcomes.
func FuzzPeersHandler(f *testing.F) {
	f.Add(hiveFrame(f, &pb.Peers{Peers: []*pb.BzzAddress{{
		Underlay:          make([]byte, 8),
		Overlay:           make([]byte, 32),
		Signature:         make([]byte, 65),
		Nonce:             make([]byte, bzz.NonceLength),
		Timestamp:         1,
		ChequebookAddress: make([]byte, 20),
	}}}))
	f.Add(hiveFrame(f, &pb.Peers{}))
	f.Add([]byte{})
	f.Add([]byte("not a length-delimited protobuf message"))

	f.Fuzz(func(t *testing.T, data []byte) {
		self := swarm.RandAddress(t)
		svc := hive.New(
			streamtest.New(),
			ab.New(mock.NewStateStore()),
			1,
			self,
			log.Noop,
			hive.Options{AllowPrivateCIDRs: true, BootnodeMode: false},
		)
		// Per-iteration cleanup: Close() closes quit and wg.Wait()s the async
		// checkAndAddPeers goroutine, so an async panic surfaces before the
		// iteration ends.
		testutil.CleanupCloser(t, svc)

		recorder := streamtest.New(
			streamtest.WithProtocols(svc.Protocol()),
			streamtest.WithBaseAddr(self),
		)

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		stream, err := recorder.NewStream(ctx, self, nil, "hive", "2.0.0", "peers")
		if err != nil {
			return
		}

		if _, err := stream.Write(data); err != nil {
			_ = stream.Reset()
			return
		}
		_ = stream.Close()
	})
}

// FuzzCheckAndAddPeersChequebook fuzzes the chequebook-gated branch of the real
// checkAndAddPeers that the permissive FuzzPeersHandler never reaches: the
// s.chequebookVerifier != nil gate (missing-chequebook rejection,
// common.BytesToAddress(bzzAddress.EthereumAddress), Verify), and the
// chequebookStorer.Put path whose write callback runs addressbook.Put under the
// registry mutex.
//
// It calls the already-exported Service.CheckAndAddPeers synchronously (no
// goroutine/stream needed) with per-field fuzzed pb.BzzAddress bytes. The
// Verifier and Storer mocks record calls and never panic, so a mock artifact
// can't be mistaken for a real defect.
//
// Invariant: must not panic. Verifier.Calls / Storer.Puts are NOT asserted:
// valid input can be dropped at the underlay/parse/timestamp gates before
// verification, so success side effects are not guaranteed.
func FuzzCheckAndAddPeersChequebook(f *testing.F) {
	cb := common.HexToAddress("0x1111111111111111111111111111111111111111")
	valid := fuzzSignedChequebookRecord(f, 1, time.Now().Unix(), cb)
	f.Add(valid.Overlay, valid.Underlay, valid.Signature, valid.Nonce, valid.Timestamp, valid.ChequebookAddress)
	f.Add([]byte{}, []byte{}, []byte{}, []byte{}, int64(0), []byte{})
	f.Add(make([]byte, 32), make([]byte, 8), make([]byte, 65), make([]byte, bzz.NonceLength), int64(1), make([]byte, 20))

	f.Fuzz(func(t *testing.T, overlay, underlay, signature, nonce []byte, ts int64, cbAddr []byte) {
		v := &chequebookmock.Verifier{Behavior: func(_, _ common.Address, _ swarm.Address, _ bool) error {
			return nil
		}}
		storer := &chequebookmock.Storer{}
		svc, _ := newHiveForTest(t, v, storer)

		svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{{
			Overlay:           overlay,
			Underlay:          underlay,
			Signature:         signature,
			Nonce:             nonce,
			Timestamp:         ts,
			ChequebookAddress: cbAddr,
		}}})
	})
}
