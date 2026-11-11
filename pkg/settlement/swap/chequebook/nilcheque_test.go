// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook_test

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	storemock "github.com/ethersphere/bee/v2/pkg/statestore/mock"
)

// TestChequeStoreLastChequeNilEntry is a regression test for NIL-02. A
// statestore entry holding a literal JSON null unmarshals into a nil
// *SignedCheque with a nil error; LastCheque must not return that nil cheque
// for a caller to dereference on cashout or a LastCheques listing, and must
// report it as corrupt rather than as no cheque.
func TestChequeStoreLastChequeNilEntry(t *testing.T) {
	t.Parallel()

	store := storemock.NewStateStore()
	chequebookAddr := common.HexToAddress("0xcb")

	// Seed the last-received-cheque key with a literal JSON null.
	if err := store.Put(chequebook.LastReceivedChequeKey(chequebookAddr), (*chequebook.SignedCheque)(nil)); err != nil {
		t.Fatal(err)
	}

	cs := chequebook.NewChequeStore(store, nil, 1, common.HexToAddress("0xbe"), nil, nil)

	cheque, err := cs.LastCheque(chequebookAddr)
	if !errors.Is(err, chequebook.ErrNilCheque) {
		t.Fatalf("expected %v, got %v", chequebook.ErrNilCheque, err)
	}
	if errors.Is(err, chequebook.ErrNoCheque) {
		t.Fatal("null entry must not be reported as no cheque")
	}
	if cheque != nil {
		t.Fatalf("expected nil cheque, got %v", cheque)
	}
}

// TestChequeStoreReceiveChequeNilLastReceived is a regression test for NIL-03.
// The last-received-cheque key can hold a literal JSON null, which unmarshals
// to a nil *SignedCheque with a nil error; ReceiveCheque must not dereference
// lastReceivedCheque.CumulativePayout, and must not treat the entry as a first
// cheque, which would accept payouts already received.
func TestChequeStoreReceiveChequeNilLastReceived(t *testing.T) {
	t.Parallel()

	store := storemock.NewStateStore()
	beneficiary := common.HexToAddress("0xbe")
	chequebookAddr := common.HexToAddress("0xcb")

	if err := store.Put(chequebook.LastReceivedChequeKey(chequebookAddr), (*chequebook.SignedCheque)(nil)); err != nil {
		t.Fatal(err)
	}

	cs := chequebook.NewChequeStore(store, nil, 1, beneficiary, nil, nil)

	cheque := &chequebook.SignedCheque{
		Beneficiary:      beneficiary,
		Chequebook:       chequebookAddr,
		CumulativePayout: big.NewInt(100),
		Signature:        []byte{},
	}

	_, err := cs.ReceiveCheque(context.Background(), cheque, big.NewInt(1), big.NewInt(0))
	if !errors.Is(err, chequebook.ErrNilCheque) {
		t.Fatalf("expected %v, got %v", chequebook.ErrNilCheque, err)
	}
	if errors.Is(err, chequebook.ErrNoCheque) {
		t.Fatal("null entry must not be reported as no cheque")
	}
}

// TestServiceLastChequeNilEntry is a regression test for NIL-04. The
// last-issued-cheque key can hold a literal JSON null, which unmarshals into a
// nil *SignedCheque with a nil error; service.LastCheque must not return that
// nil cheque for the caller to dereference. It must not report ErrNoCheque
// either, or Issue would start the next cheque from zero.
func TestServiceLastChequeNilEntry(t *testing.T) {
	t.Parallel()

	store := storemock.NewStateStore()
	beneficiary := common.HexToAddress("0xbe")

	if err := store.Put(chequebook.LastIssuedChequeKey(beneficiary), (*chequebook.SignedCheque)(nil)); err != nil {
		t.Fatal(err)
	}

	svc, err := chequebook.New(nil, common.Address{}, common.Address{}, store, nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	cheque, err := svc.LastCheque(beneficiary)
	if !errors.Is(err, chequebook.ErrNilCheque) {
		t.Fatalf("expected %v, got %v", chequebook.ErrNilCheque, err)
	}
	if errors.Is(err, chequebook.ErrNoCheque) {
		t.Fatal("null entry must not be reported as no cheque")
	}
	if cheque != nil {
		t.Fatalf("expected nil cheque, got %v", cheque)
	}
}

// TestServiceLastChequeNilCumulativePayout covers a last-issued-cheque entry of
// `{}`: it unmarshals into a non-nil cheque whose CumulativePayout is nil.
// LastCheque must reject it with its own error, not ErrNoCheque, so Issue does
// not treat the row as a first cheque and start the payout at zero.
func TestServiceLastChequeNilCumulativePayout(t *testing.T) {
	t.Parallel()

	store := newRawStore()
	beneficiary := common.HexToAddress("0xbe")
	store.setRaw(chequebook.LastIssuedChequeKey(beneficiary), []byte(`{}`))

	svc, err := chequebook.New(nil, common.Address{}, common.Address{}, store, nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	cheque, err := svc.LastCheque(beneficiary)
	if !errors.Is(err, chequebook.ErrNilCumulativePayout) {
		t.Fatalf("expected %v, got %v", chequebook.ErrNilCumulativePayout, err)
	}
	if errors.Is(err, chequebook.ErrNoCheque) {
		t.Fatal("corrupt payout must not be reported as no cheque")
	}
	if cheque != nil {
		t.Fatalf("expected nil cheque, got %v", cheque)
	}
}

// TestChequeStoreLastChequeNilCumulativePayout covers a last-received-cheque
// entry of `{}`. Readers such as settlements add CumulativePayout, so LastCheque
// must reject the blank amount before it is returned.
func TestChequeStoreLastChequeNilCumulativePayout(t *testing.T) {
	t.Parallel()

	store := newRawStore()
	chequebookAddr := common.HexToAddress("0xcb")
	store.setRaw(chequebook.LastReceivedChequeKey(chequebookAddr), []byte(`{}`))

	cs := chequebook.NewChequeStore(store, nil, 1, common.HexToAddress("0xbe"), nil, nil)

	cheque, err := cs.LastCheque(chequebookAddr)
	if !errors.Is(err, chequebook.ErrNilCumulativePayout) {
		t.Fatalf("expected %v, got %v", chequebook.ErrNilCumulativePayout, err)
	}
	if errors.Is(err, chequebook.ErrNoCheque) {
		t.Fatal("corrupt payout must not be reported as no cheque")
	}
	if cheque != nil {
		t.Fatalf("expected nil cheque, got %v", cheque)
	}
}

// TestChequeStoreReceiveChequeNilCumulativePayout covers a cheque with a nil
// CumulativePayout passed to ReceiveCheque directly, which big.Int.Sub would
// dereference. ReceiveCheque must reject it instead of panicking. A
// peer-supplied cheque that omits cumulativePayout is already rejected by
// SignedCheque.UnmarshalJSON; that path is covered in cheque_json_test.go.
func TestChequeStoreReceiveChequeNilCumulativePayout(t *testing.T) {
	t.Parallel()

	beneficiary := common.HexToAddress("0xbe")
	chequebookAddr := common.HexToAddress("0xcb")

	cheque := &chequebook.SignedCheque{
		Beneficiary: beneficiary,
		Chequebook:  chequebookAddr,
		Signature:   []byte{0},
	}

	cs := chequebook.NewChequeStore(storemock.NewStateStore(), nil, 1, beneficiary, nil, nil)

	_, err := cs.ReceiveCheque(context.Background(), cheque, big.NewInt(1), big.NewInt(0))
	if !errors.Is(err, chequebook.ErrChequeInvalid) {
		t.Fatalf("expected %v, got %v", chequebook.ErrChequeInvalid, err)
	}
}

// TestChequeStoreEmptyObjectLastReceived is a regression test for a
// last-received-cheque entry holding an empty JSON object: it unmarshals into a
// non-nil cheque whose CumulativePayout is a nil *big.Int, which the incoming
// cheque comparison would dereference. ReceiveCheque must reject it instead of
// panicking, and LastCheque must never report success with a nil cheque.
func TestChequeStoreEmptyObjectLastReceived(t *testing.T) {
	t.Parallel()

	store := newRawStore()
	beneficiary := common.HexToAddress("0xbe")
	chequebookAddr := common.HexToAddress("0xcb")

	store.setRaw(chequebook.LastReceivedChequeKey(chequebookAddr), []byte("{}"))

	cs := chequebook.NewChequeStore(store, nil, 1, beneficiary, nil, nil)

	lastCheque, err := cs.LastCheque(chequebookAddr)
	if !errors.Is(err, chequebook.ErrNilCumulativePayout) {
		t.Fatalf("expected %v, got %v", chequebook.ErrNilCumulativePayout, err)
	}
	if errors.Is(err, chequebook.ErrNoCheque) {
		t.Fatal("corrupt payout must not be reported as no cheque")
	}
	if lastCheque != nil {
		t.Fatalf("expected nil cheque, got %v", lastCheque)
	}

	cheque := &chequebook.SignedCheque{
		Beneficiary:      beneficiary,
		Chequebook:       chequebookAddr,
		CumulativePayout: big.NewInt(100),
		Signature:        make([]byte, 65),
	}

	if _, err := cs.ReceiveCheque(context.Background(), cheque, big.NewInt(1), big.NewInt(0)); err == nil {
		t.Fatal("expected an error for a corrupted last received cheque")
	}
}
