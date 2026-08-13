// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	storemock "github.com/ethersphere/bee/v2/pkg/statestore/mock"
)

// TestChequeStoreLastChequeNilEntry is a regression test for NIL-02. A
// statestore entry holding a literal JSON null unmarshals into a nil
// *SignedCheque with a nil error; LastCheque must not return that nil cheque
// for a caller to dereference on cashout or a LastCheques listing.
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
	if !errors.Is(err, chequebook.ErrNoCheque) {
		t.Fatalf("expected %v, got %v", chequebook.ErrNoCheque, err)
	}
	if cheque != nil {
		t.Fatalf("expected nil cheque, got %v", cheque)
	}
}

// TestChequeStoreReceiveChequeNilLastReceived is a regression test for NIL-03.
// The last-received-cheque key can hold a literal JSON null, which unmarshals
// to a nil *SignedCheque with a nil error; ReceiveCheque must not dereference
// lastReceivedCheque.CumulativePayout.
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
		Cheque: chequebook.Cheque{
			Beneficiary:      beneficiary,
			Chequebook:       chequebookAddr,
			CumulativePayout: big.NewInt(100),
		},
		Signature: []byte{},
	}

	_, err := cs.ReceiveCheque(context.Background(), cheque, big.NewInt(1), big.NewInt(0))
	if !errors.Is(err, chequebook.ErrNoCheque) {
		t.Fatalf("expected %v, got %v", chequebook.ErrNoCheque, err)
	}
}

// TestServiceLastChequeNilEntry is a regression test for NIL-04. The
// last-issued-cheque key can hold a literal JSON null, which unmarshals into a
// nil *SignedCheque with a nil error; service.LastCheque must not return that
// nil cheque for the caller to dereference.
func TestServiceLastChequeNilEntry(t *testing.T) {
	t.Parallel()

	store := storemock.NewStateStore()
	beneficiary := common.HexToAddress("0xbe")

	if err := store.Put(chequebook.LastIssuedChequeKey(beneficiary), (*chequebook.SignedCheque)(nil)); err != nil {
		t.Fatal(err)
	}

	svc, err := chequebook.New(nil, common.Address{}, common.Address{}, store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	cheque, err := svc.LastCheque(beneficiary)
	if !errors.Is(err, chequebook.ErrNoCheque) {
		t.Fatalf("expected %v, got %v", chequebook.ErrNoCheque, err)
	}
	if cheque != nil {
		t.Fatalf("expected nil cheque, got %v", cheque)
	}
}

// TestChequeStoreReceiveChequeNilCumulativePayout is a regression test for a
// peer-supplied cheque that simply omits cumulativePayout: it unmarshals into a
// non-nil cheque whose CumulativePayout is a nil *big.Int, which big.Int.Sub
// dereferences. ReceiveCheque must reject it instead of panicking.
func TestChequeStoreReceiveChequeNilCumulativePayout(t *testing.T) {
	t.Parallel()

	beneficiary := common.HexToAddress("0xbe")
	chequebookAddr := common.HexToAddress("0xcb")

	var cheque *chequebook.SignedCheque
	body := fmt.Sprintf(`{"beneficiary":"%s","chequebook":"%s","signature":"AAA="}`, beneficiary, chequebookAddr)
	if err := json.Unmarshal([]byte(body), &cheque); err != nil {
		t.Fatal(err)
	}
	if cheque == nil || cheque.CumulativePayout != nil {
		t.Fatal("expected a non-nil cheque with a nil cumulative payout")
	}

	cs := chequebook.NewChequeStore(storemock.NewStateStore(), nil, 1, beneficiary, nil, nil)

	_, err := cs.ReceiveCheque(context.Background(), cheque, big.NewInt(1), big.NewInt(0))
	if !errors.Is(err, chequebook.ErrChequeInvalid) {
		t.Fatalf("expected %v, got %v", chequebook.ErrChequeInvalid, err)
	}
}
