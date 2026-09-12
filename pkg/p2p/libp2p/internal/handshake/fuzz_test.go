// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/handshake"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/handshake/pb"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// handshakeFrame length-delimited encodes a protobuf message the same way the
// production writer does, so a fuzz seed decodes cleanly through the reader.
func handshakeFrame(tb testing.TB, msg protobuf.Message) []byte {
	tb.Helper()
	var buf bytes.Buffer
	if err := protobuf.NewWriter(&buf).WriteMsg(msg); err != nil {
		tb.Fatal(err)
	}
	return buf.Bytes()
}

// fuzzUnderlayBytes serializes a single valid /ip4.../tcp/.../p2p/... underlay,
// producing the peer-supplied ObservedUnderlay bytes a real Syn carries.
func fuzzUnderlayBytes(tb testing.TB) []byte {
	tb.Helper()
	m, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA")
	if err != nil {
		tb.Fatal(err)
	}
	b, err := bzz.SerializeUnderlays([]ma.Multiaddr{m})
	if err != nil {
		tb.Fatal(err)
	}
	return b
}

// FuzzSynRead drives arbitrary bytes through the exact read path Handle uses
// for the first peer message (handshake.go:336-345): decode a pb.Syn via the
// production reader, then hand its one peer-controlled field to the real
// DeserializeUnderlays parse. Asserts the combined path never panics.
func FuzzSynRead(f *testing.F) {
	f.Add(handshakeFrame(f, &pb.Syn{ObservedUnderlay: fuzzUnderlayBytes(f)}))
	f.Add(handshakeFrame(f, &pb.Syn{}))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := protobuf.NewReader(bytes.NewReader(data))
		var m pb.Syn
		if err := r.ReadMsg(&m); err != nil {
			return
		}
		// Mirror Handle: parse the observed underlay bytes. Must return
		// (result, nil) or (_, err) without panicking on arbitrary input.
		_, _ = bzz.DeserializeUnderlays(m.ObservedUnderlay)
	})
}

// FuzzAckRead drives arbitrary bytes through the read path Handle uses for the
// second peer message (handshake.go:428-439): decode a pb.Ack, apply the exact
// nil-guard on the nested proto3-optional BzzAddress pointer, then touch every
// field. Confirms an absent nested pointer does not panic on field access.
func FuzzAckRead(f *testing.F) {
	f.Add(handshakeFrame(f, &pb.Ack{
		Address: &pb.BzzAddress{
			Underlay:          fuzzUnderlayBytes(f),
			Overlay:           make([]byte, swarm.HashSize),
			Signature:         make([]byte, 65),
			Nonce:             make([]byte, swarm.HashSize),
			Timestamp:         1700000000,
			ChequebookAddress: make([]byte, common.AddressLength),
		},
		NetworkID:      3,
		FullNode:       true,
		WelcomeMessage: "hello",
	}))
	// Decodable Ack with an absent nested BzzAddress pointer.
	f.Add(handshakeFrame(f, &pb.Ack{NetworkID: 3, FullNode: true}))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := protobuf.NewReader(bytes.NewReader(data))
		var m pb.Ack
		if err := r.ReadMsg(&m); err != nil {
			return
		}
		if m.Address == nil {
			return
		}
		_ = m.Address.Overlay
		_ = m.Address.Underlay
		_ = m.Address.Signature
		_ = m.Address.Nonce
		_ = m.Address.Timestamp
		_ = m.Address.ChequebookAddress
	})
}

// FuzzSynAckRead drives arbitrary bytes through the read path the initiator
// uses (Handshake, handshake.go:212-229): decode a pb.SynAck, apply the exact
// nil-guard sequence over its proto3-optional nested pointers, then parse the
// peer-supplied observed underlay bytes. Reproduces the nil-nested-pointer
// hazard the code comments call out.
func FuzzSynAckRead(f *testing.F) {
	valid := &pb.SynAck{
		Syn: &pb.Syn{ObservedUnderlay: fuzzUnderlayBytes(f)},
		Ack: &pb.Ack{
			Address: &pb.BzzAddress{
				Underlay:  fuzzUnderlayBytes(f),
				Overlay:   make([]byte, swarm.HashSize),
				Signature: make([]byte, 65),
				Nonce:     make([]byte, swarm.HashSize),
				Timestamp: 1700000000,
			},
			NetworkID: 3,
			FullNode:  true,
		},
	}
	f.Add(handshakeFrame(f, valid))
	f.Add(handshakeFrame(f, &pb.SynAck{Ack: valid.Ack}))                             // Syn == nil
	f.Add(handshakeFrame(f, &pb.SynAck{Syn: valid.Syn}))                             // Ack == nil
	f.Add(handshakeFrame(f, &pb.SynAck{Syn: valid.Syn, Ack: &pb.Ack{NetworkID: 3}})) // Ack.Address == nil
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := protobuf.NewReader(bytes.NewReader(data))
		var resp pb.SynAck
		if err := r.ReadMsg(&resp); err != nil {
			return
		}
		if resp.Syn == nil {
			return
		}
		if resp.Ack == nil || resp.Ack.Address == nil {
			return
		}
		_, _ = bzz.DeserializeUnderlays(resp.Syn.ObservedUnderlay)
	})
}

// newFuzzService builds a handshake.Service wired to a fixed clock, mirroring
// newTimestampTestService (which takes *testing.T) so it can be built from a
// *testing.F seed context. chequebookVerifier is nil, so parseCheckAck makes no
// blockchain calls even for FullNode acks.
func newFuzzService(tb testing.TB, networkID uint64, now time.Time) *handshake.Service {
	tb.Helper()

	pk, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		tb.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(pk)
	nonce := common.HexToHash("0x1").Bytes()

	overlay, err := crypto.NewOverlayAddress(pk.PublicKey, networkID, nonce)
	if err != nil {
		tb.Fatal(err)
	}

	m, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA")
	if err != nil {
		tb.Fatal(err)
	}
	infos, err := libp2ppeer.AddrInfosFromP2pAddrs(m)
	if err != nil {
		tb.Fatal(err)
	}

	svc, err := handshake.New(signer, resolveIdentity{}, overlay, networkID, true, nonce, nil, "", noopAddressbook{}, infos[0].ID, nil, log.Noop)
	if err != nil {
		tb.Fatal(err)
	}
	svc.SetTime(func() time.Time { return now })
	return svc
}

// signedProtoAck builds a fully valid signed pb.Ack for a fresh peer identity
// at the given timestamp, reusing handshakeSigningBytes so the recovered
// overlay matches the supplied one. Chequebook is empty (matches the signing
// bytes).
func signedProtoAck(tb testing.TB, networkID uint64, ts int64) *pb.Ack {
	tb.Helper()

	pk, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		tb.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(pk)
	nonce := common.HexToHash("0x2").Bytes()

	overlay, err := crypto.NewOverlayAddress(pk.PublicKey, networkID, nonce)
	if err != nil {
		tb.Fatal(err)
	}

	u, err := ma.NewMultiaddr("/ip4/10.0.0.5/tcp/7070")
	if err != nil {
		tb.Fatal(err)
	}
	underlaysBytes, err := bzz.SerializeUnderlays([]ma.Multiaddr{u})
	if err != nil {
		tb.Fatal(err)
	}

	sig, err := signer.Sign(handshakeSigningBytes(underlaysBytes, overlay.Bytes(), networkID, nonce, ts))
	if err != nil {
		tb.Fatal(err)
	}

	return &pb.Ack{
		Address: &pb.BzzAddress{
			Underlay:  underlaysBytes,
			Overlay:   overlay.Bytes(),
			Signature: sig,
			Nonce:     nonce,
			Timestamp: ts,
		},
		NetworkID: networkID,
		FullNode:  true,
	}
}

// FuzzParseCheckAck drives peer-controlled bytes through the deepest handshake
// trust-boundary logic: decode a pb.Ack via the production reader, then call
// the real parseCheckAck (bzz.ParseAddress signature recovery + overlay match +
// chequebook-length guard + underlay deserialize, plus bzz.CheckTimestamp). The
// service uses a fixed clock and a nil chequebookVerifier, so the path is
// deterministic and makes no blockchain calls.
func FuzzParseCheckAck(f *testing.F) {
	const networkID = uint64(7)
	now := time.Unix(1700000000, 0)

	// Valid, fully signed ack at the fixed clock time -> parseCheckAck succeeds.
	f.Add(handshakeFrame(f, signedProtoAck(f, networkID, now.Unix())))
	// Bad signature: valid shape, corrupted signature -> overlay mismatch.
	badSig := signedProtoAck(f, networkID, now.Unix())
	badSig.Address.Signature = make([]byte, len(badSig.Address.Signature))
	f.Add(handshakeFrame(f, badSig))
	// Wrong-length chequebook (guard rejects).
	badCb := signedProtoAck(f, networkID, now.Unix())
	badCb.Address.ChequebookAddress = []byte{0x01, 0x02, 0x03}
	f.Add(handshakeFrame(f, badCb))
	// Empty overlay.
	emptyOverlay := signedProtoAck(f, networkID, now.Unix())
	emptyOverlay.Address.Overlay = nil
	f.Add(handshakeFrame(f, emptyOverlay))
	f.Add([]byte{})

	svc := newFuzzService(f, networkID, now)

	f.Fuzz(func(t *testing.T, data []byte) {
		r := protobuf.NewReader(bytes.NewReader(data))
		var ack pb.Ack
		if err := r.ReadMsg(&ack); err != nil {
			return
		}
		if ack.Address == nil {
			return
		}

		addr, err := svc.ParseCheckAck(context.Background(), &ack)
		if err != nil {
			return
		}
		// On success, bzz.ParseAddress only returns when the recovered
		// overlay equals the supplied one; a success violating this is a
		// real defect.
		if addr == nil {
			t.Fatal("parseCheckAck returned nil address without error")
		}
		if !bytes.Equal(addr.Overlay.Bytes(), ack.Address.Overlay) {
			t.Fatalf("overlay mismatch on success: got %x, want %x", addr.Overlay.Bytes(), ack.Address.Overlay)
		}
	})
}
