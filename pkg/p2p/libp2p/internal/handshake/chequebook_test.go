// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake_test

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/handshake"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/handshake/pb"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	chequebookmock "github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// signProtoAckWithChequebook builds a signed pb.Ack carrying the given
// chequebook address. Signing includes the chequebook so ParseAddress
// produces an EthereumAddress that matches the signer key.
func signProtoAckWithChequebook(t *testing.T, networkID uint64, ts int64, cb common.Address) *pb.Ack {
	t.Helper()

	pk, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(pk)
	nonce := common.HexToHash("0x2").Bytes()

	overlay, err := crypto.NewOverlayAddress(pk.PublicKey, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}

	u, err := ma.NewMultiaddr("/ip4/10.0.0.5/tcp/7070")
	if err != nil {
		t.Fatal(err)
	}
	underlayBytes, err := bzz.SerializeUnderlays([]ma.Multiaddr{u})
	if err != nil {
		t.Fatal(err)
	}

	networkIDBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(networkIDBytes, networkID)
	tsBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(tsBytes, uint64(ts))
	signData := append([]byte("bee-handshake-"), underlayBytes...)
	signData = append(signData, overlay.Bytes()...)
	signData = append(signData, networkIDBytes...)
	signData = append(signData, nonce...)
	signData = append(signData, tsBytes...)
	signData = append(signData, cb.Bytes()...)

	sig, err := signer.Sign(signData)
	if err != nil {
		t.Fatal(err)
	}

	return &pb.Ack{
		Address: &pb.BzzAddress{
			Underlay:          underlayBytes,
			Overlay:           overlay.Bytes(),
			Signature:         sig,
			Nonce:             nonce,
			Timestamp:         ts,
			ChequebookAddress: cb.Bytes(),
		},
		NetworkID: networkID,
		FullNode:  true,
	}
}

// newChequebookTestService builds a handshake.Service wired with the given
// verifier, fixed clock, and empty addressbook.
func newChequebookTestService(t *testing.T, networkID uint64, now time.Time, verifier chequebook.Verifier) *handshake.Service {
	t.Helper()
	return newChequebookTestServiceWithBook(t, networkID, now, verifier, noopAddressbook{})
}

// newChequebookTestServiceWithBook is the same as newChequebookTestService but
// lets a test inject a custom addressbook.Getter.
func newChequebookTestServiceWithBook(t *testing.T, networkID uint64, now time.Time, verifier chequebook.Verifier, book addressbook.Getter) *handshake.Service {
	t.Helper()

	pk, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(pk)
	nonce := common.HexToHash("0x1").Bytes()

	overlay, err := crypto.NewOverlayAddress(pk.PublicKey, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}

	m, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA")
	if err != nil {
		t.Fatal(err)
	}
	infos, err := libp2ppeer.AddrInfosFromP2pAddrs(m)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := handshake.New(signer, resolveIdentity{}, overlay, networkID, true, nonce, nil, "", book, infos[0].ID, verifier, log.Noop)
	if err != nil {
		t.Fatal(err)
	}
	svc.SetTime(func() time.Time { return now })
	return svc
}

// stubAddressbook implements addressbook.Getter and returns a fixed entry.
type stubAddressbook struct {
	addr     *bzz.Address
	verified bool
}

func (s stubAddressbook) Get(_ swarm.Address) (*bzz.Address, bool, error) {
	if s.addr == nil {
		return nil, false, addressbook.ErrNotFound
	}
	return s.addr, s.verified, nil
}

func TestParseCheckAck_NoVerifierAcceptsAnyChequebook(t *testing.T) {
	t.Parallel()

	networkID := uint64(7)
	now := time.Unix(1700000000, 0)
	svc := newChequebookTestService(t, networkID, now, nil)

	ack := signProtoAckWithChequebook(t, networkID, now.Unix(), common.Address{})
	if _, err := svc.ParseCheckAck(context.Background(), ack); err != nil {
		t.Fatalf("nil verifier must accept any record, got %v", err)
	}
}

func TestParseCheckAck_VerifierRejectsMissingChequebook(t *testing.T) {
	t.Parallel()

	networkID := uint64(7)
	now := time.Unix(1700000000, 0)
	v := &chequebookmock.Verifier{}
	svc := newChequebookTestService(t, networkID, now, v)

	ack := signProtoAckWithChequebook(t, networkID, now.Unix(), common.Address{})
	if _, err := svc.ParseCheckAck(context.Background(), ack); !errors.Is(err, chequebook.ErrChequebookAddressMissing) {
		t.Fatalf("expected ErrChequebookAddressMissing for empty chequebook, got %v", err)
	}
	if v.Calls != 0 {
		t.Fatalf("verifier must not run when chequebook is missing, got calls=%d", v.Calls)
	}
}

func TestParseCheckAck_VerifierRejectsBadIssuer(t *testing.T) {
	t.Parallel()

	networkID := uint64(7)
	now := time.Unix(1700000000, 0)
	v := &chequebookmock.Verifier{Behavior: func(_, _ common.Address, _ swarm.Address, _ bool) error {
		return chequebook.ErrChequebookIssuerMismatch
	}}
	svc := newChequebookTestService(t, networkID, now, v)

	cb := common.HexToAddress("0x1111111111111111111111111111111111111111")
	ack := signProtoAckWithChequebook(t, networkID, now.Unix(), cb)
	if _, err := svc.ParseCheckAck(context.Background(), ack); !errors.Is(err, chequebook.ErrChequebookIssuerMismatch) {
		t.Fatalf("expected ErrChequebookIssuerMismatch on issuer mismatch, got %v", err)
	}
	if v.Calls != 1 {
		t.Fatalf("verifier must be called once, got %d", v.Calls)
	}
}

func TestParseCheckAck_VerifierAccepts(t *testing.T) {
	t.Parallel()

	networkID := uint64(7)
	now := time.Unix(1700000000, 0)
	cb := common.HexToAddress("0x1111111111111111111111111111111111111111")

	var gotCB common.Address
	v := &chequebookmock.Verifier{Behavior: func(c, _ common.Address, _ swarm.Address, _ bool) error {
		gotCB = c
		return nil
	}}
	svc := newChequebookTestService(t, networkID, now, v)

	ack := signProtoAckWithChequebook(t, networkID, now.Unix(), cb)
	bzzAddr, err := svc.ParseCheckAck(context.Background(), ack)
	if err != nil {
		t.Fatalf("verifier must accept valid record, got %v", err)
	}
	if bzzAddr.ChequebookAddress != cb {
		t.Fatalf("returned record carries cb %s, want %s", bzzAddr.ChequebookAddress.Hex(), cb.Hex())
	}
	if gotCB != cb {
		t.Fatalf("verifier got cb %s, want %s", gotCB.Hex(), cb.Hex())
	}
	if v.Calls != 1 {
		t.Fatalf("verifier must be called once, got %d", v.Calls)
	}
}

// TestParseCheckAck_PastVerifiedFalseForUnknownPeer asserts the verifier
// receives pastVerified=false when the addressbook has no entry for the peer.
func TestParseCheckAck_PastVerifiedFalseForUnknownPeer(t *testing.T) {
	t.Parallel()

	networkID := uint64(7)
	now := time.Unix(1700000000, 0)
	cb := common.HexToAddress("0x1111111111111111111111111111111111111111")

	var got []bool
	v := &chequebookmock.Verifier{Behavior: func(_, _ common.Address, _ swarm.Address, pastVerified bool) error {
		got = append(got, pastVerified)
		return nil
	}}
	svc := newChequebookTestService(t, networkID, now, v)

	ack := signProtoAckWithChequebook(t, networkID, now.Unix(), cb)
	if _, err := svc.ParseCheckAck(context.Background(), ack); err != nil {
		t.Fatalf("ParseCheckAck: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("verifier calls: got %d, want 1", len(got))
	}
	if got[0] {
		t.Fatalf("pastVerified: got true, want false for unknown peer")
	}
}

// TestParseCheckAck_PastVerifiedTrueForVerifiedEntry asserts the verifier
// receives pastVerified=true when the addressbook entry was previously stored
// with Verified=true.
func TestParseCheckAck_PastVerifiedTrueForVerifiedEntry(t *testing.T) {
	t.Parallel()

	networkID := uint64(7)
	now := time.Unix(1700000000, 0)
	cb := common.HexToAddress("0x1111111111111111111111111111111111111111")

	// Existing entry with a timestamp old enough that the fresh ack passes
	// CheckTimestamp's min-interval gate, and with a matching chequebook.
	stub := stubAddressbook{
		addr: &bzz.Address{
			Timestamp:         now.Unix() - int64(bzz.MinimumUpdateInterval.Seconds()) - 1,
			ChequebookAddress: cb,
		},
		verified: true,
	}

	var got []bool
	v := &chequebookmock.Verifier{Behavior: func(_, _ common.Address, _ swarm.Address, pastVerified bool) error {
		got = append(got, pastVerified)
		return nil
	}}
	svc := newChequebookTestServiceWithBook(t, networkID, now, v, stub)

	ack := signProtoAckWithChequebook(t, networkID, now.Unix(), cb)
	if _, err := svc.ParseCheckAck(context.Background(), ack); err != nil {
		t.Fatalf("ParseCheckAck: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("verifier calls: got %d, want 1", len(got))
	}
	if !got[0] {
		t.Fatalf("pastVerified: got false, want true for previously-verified entry with matching chequebook")
	}
}

// TestParseCheckAck_PastVerifiedFalseOnChequebookRotation asserts that the
// stored Verified=true flag is NOT honoured when the incoming record carries
// a different chequebook address than the stored one. Without this, a peer
// could rotate to a chequebook they do not own and have the on-chain issuer
// and bytecode checks wrongly skipped.
func TestParseCheckAck_PastVerifiedFalseOnChequebookRotation(t *testing.T) {
	t.Parallel()

	networkID := uint64(7)
	now := time.Unix(1700000000, 0)
	oldCB := common.HexToAddress("0x1111111111111111111111111111111111111111")
	newCB := common.HexToAddress("0x2222222222222222222222222222222222222222")

	stub := stubAddressbook{
		addr: &bzz.Address{
			Timestamp:         now.Unix() - int64(bzz.MinimumUpdateInterval.Seconds()) - 1,
			ChequebookAddress: oldCB,
		},
		verified: true,
	}

	var got []bool
	v := &chequebookmock.Verifier{Behavior: func(_, _ common.Address, _ swarm.Address, pastVerified bool) error {
		got = append(got, pastVerified)
		return nil
	}}
	svc := newChequebookTestServiceWithBook(t, networkID, now, v, stub)

	ack := signProtoAckWithChequebook(t, networkID, now.Unix(), newCB)
	if _, err := svc.ParseCheckAck(context.Background(), ack); err != nil {
		t.Fatalf("ParseCheckAck: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("verifier calls: got %d, want 1", len(got))
	}
	if got[0] {
		t.Fatalf("pastVerified: got true, want false when chequebook rotates")
	}
}

// TestParseCheckAck_VerifierSkippedForLightPeer asserts the chequebook
// verifier is NOT invoked when the peer advertises FullNode=false. Per
// issue #5, the chequebook gate is scoped to full-node peers. Ultra-light
// peers have no chain so cannot deploy a chequebook; light peers may
// have one but are still out of scope — the gate is keyed on peer mode,
// not on chequebook presence.
func TestParseCheckAck_VerifierSkippedForLightPeer(t *testing.T) {
	t.Parallel()

	networkID := uint64(7)
	now := time.Unix(1700000000, 0)

	var called int
	v := &chequebookmock.Verifier{Behavior: func(_, _ common.Address, _ swarm.Address, _ bool) error {
		called++
		return nil
	}}
	svc := newChequebookTestService(t, networkID, now, v)

	// Light peer: FullNode=false, zero chequebook.
	ack := signProtoAckWithChequebook(t, networkID, now.Unix(), common.Address{})
	ack.FullNode = false
	if _, err := svc.ParseCheckAck(context.Background(), ack); err != nil {
		t.Fatalf("ParseCheckAck must accept a light peer with no chequebook, got %v", err)
	}
	if called != 0 {
		t.Fatalf("verifier must not run for a light peer, calls=%d", called)
	}
}
