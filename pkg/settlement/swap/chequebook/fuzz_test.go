// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	storemock "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	transactionmock "github.com/ethersphere/bee/v2/pkg/transaction/mock"
)

// FuzzRecoverCheque fuzzes the EIP-712 issuer recovery that verifies a peer's
// cheque signature. Chequebook/beneficiary addresses, the cumulative payout, and
// the signature are all peer-controlled, so the target asserts the recovery
// never panics on hostile input (including a nil CumulativePayout, which the
// EIP-712 encoder must tolerate).
func FuzzRecoverCheque(f *testing.F) {
	f.Add(make([]byte, 20), make([]byte, 20), big.NewInt(101).Bytes(), make([]byte, 65), int64(1))
	f.Add([]byte{}, []byte{}, []byte{}, []byte{}, int64(1))

	f.Fuzz(func(t *testing.T, chequebookAddr, beneficiary, payout, sig []byte, chainID int64) {
		var cp *big.Int
		if len(payout) > 0 {
			cp = new(big.Int).SetBytes(payout)
		}
		cheque := &chequebook.SignedCheque{
			Cheque: chequebook.Cheque{
				Chequebook:       common.BytesToAddress(chequebookAddr),
				Beneficiary:      common.BytesToAddress(beneficiary),
				CumulativePayout: cp,
			},
			Signature: sig,
		}
		_, _ = chequebook.RecoverCheque(cheque, chainID)
	})
}

// FuzzReceiveCheque drives peer-controlled cheque contents through the real
// chequeStore.ReceiveCheque verification. The exchange rate is set very high so
// any well-formed cheque is rejected as too low BEFORE any blockchain call,
// isolating the target on the pre-signature validation arithmetic. An empty
// payout models a peer cheque whose JSON omitted CumulativePayout (decoding to a
// nil *big.Int) — exactly what the swap handler forwards after json.Unmarshal.
func FuzzReceiveCheque(f *testing.F) {
	beneficiary := common.HexToAddress("0xffff")

	// huge exchange rate so non-nil payouts return ErrChequeValueTooLow before
	// touching the transaction service.
	exchangeRate := new(big.Int).Lsh(big.NewInt(1), 240)
	deduction := big.NewInt(0)

	newStore := func() chequebook.ChequeStore {
		return chequebook.NewChequeStore(
			storemock.NewStateStore(),
			&factoryMock{verifyChequebook: func(context.Context, common.Address) error { return nil }},
			int64(1),
			beneficiary,
			transactionmock.New(),
			func(*chequebook.SignedCheque, int64) (common.Address, error) { return common.Address{}, nil },
		)
	}

	f.Add(make([]byte, 20), big.NewInt(101).Bytes())
	f.Add(make([]byte, 20), big.NewInt(1).Bytes())

	f.Fuzz(func(t *testing.T, chequebookAddr, payout []byte) {
		var cp *big.Int
		if len(payout) > 0 {
			cp = new(big.Int).SetBytes(payout)
		}
		cheque := &chequebook.SignedCheque{
			Cheque: chequebook.Cheque{
				Chequebook:       common.BytesToAddress(chequebookAddr),
				Beneficiary:      beneficiary,
				CumulativePayout: cp,
			},
			Signature: make([]byte, 65),
		}
		_, _ = newStore().ReceiveCheque(context.Background(), cheque, exchangeRate, deduction)
	})
}
