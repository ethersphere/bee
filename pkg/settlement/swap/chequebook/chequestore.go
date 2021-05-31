// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/transaction"
)

var (
	// ErrNoCheque is the error returned if there is no prior cheque for a chequebook or beneficiary.
	ErrNoCheque = errors.New("no cheque")
	// ErrChequeNotIncreasing is the error returned if the cheque amount is the same or lower.
	ErrChequeNotIncreasing = errors.New("cheque cumulativePayout is not increasing")
	// ErrChequeInvalid is the error returned if the cheque itself is invalid.
	ErrChequeInvalid = errors.New("invalid cheque")
	// ErrWrongBeneficiary is the error returned if the cheque has the wrong beneficiary.
	ErrWrongBeneficiary = errors.New("wrong beneficiary")
	// ErrBouncingCheque is the error returned if the chequebook is demonstrably illiquid.
	ErrBouncingCheque = errors.New("bouncing cheque")
	// ErrChequeValueTooLow is the error returned if the after deduction value of a cheque did not cover 1 accounting credit
	ErrChequeValueTooLow = errors.New("cheque value lower than acceptable")
	//
	lastReceivedChequePrefix = "swap_chequebook_last_received_cheque_"
)

// ChequeStore handles the verification and storage of received cheques
type ChequeStore interface {
	// ReceiveCheque verifies and stores a cheque. It returns the total amount earned.
	ReceiveCheque(ctx context.Context, cheque *SignedCheque, exchange *big.Int, deduction *big.Int) (*big.Int, error)
	// LastCheque returns the last cheque we received from a specific chequebook.
	LastCheque(chequebook common.Address) (*SignedCheque, error)
	// LastCheques returns the last received cheques from every known chequebook.
	LastCheques() (map[common.Address]*SignedCheque, error)
}

type chequeStore struct {
	lock               sync.Mutex
	store              storage.StateStorer
	factory            Factory
	chaindID           int64
	transactionService transaction.Service
	beneficiary        common.Address // the beneficiary we expect in cheques sent to us
	recoverChequeFunc  RecoverChequeFunc
}

type RecoverChequeFunc func(cheque *SignedCheque, chainID int64) (common.Address, error)

// NewChequeStore creates new ChequeStore
func NewChequeStore(
	store storage.StateStorer,
	factory Factory,
	chainID int64,
	beneficiary common.Address,
	transactionService transaction.Service,
	recoverChequeFunc RecoverChequeFunc) ChequeStore {
	return &chequeStore{
		store:              store,
		factory:            factory,
		chaindID:           chainID,
		transactionService: transactionService,
		beneficiary:        beneficiary,
		recoverChequeFunc:  recoverChequeFunc,
	}
}

// lastReceivedChequeKey computes the key where to store the last cheque received from a chequebook.
func lastReceivedChequeKey(chequebook common.Address) string {
	return fmt.Sprintf("%s_%x", lastReceivedChequePrefix, chequebook)
}

// LastCheque returns the last cheque we received from a specific chequebook.
func (s *chequeStore) LastCheque(chequebook common.Address) (*SignedCheque, error) {
	var cheque *SignedCheque
	err := s.store.Get(lastReceivedChequeKey(chequebook), &cheque)
	if err != nil {
		if err != storage.ErrNotFound {
			return nil, err
		}
		return nil, ErrNoCheque
	}

	return cheque, nil
}

// ReceiveCheque verifies and stores a cheque. It returns the totam amount earned.
func (s *chequeStore) ReceiveCheque(ctx context.Context, cheque *SignedCheque, exchange *big.Int, deduction *big.Int) (*big.Int, error) {
	// verify we are the beneficiary
	if cheque.Beneficiary != s.beneficiary {
		return nil, ErrWrongBeneficiary
	}

	// don't allow concurrent processing of cheques
	// this would be sufficient on a per chequebook basis
	s.lock.Lock()
	defer s.lock.Unlock()

	// load the lastCumulativePayout for the cheques chequebook
	var lastCumulativePayout *big.Int
	var lastReceivedCheque *SignedCheque
	err := s.store.Get(lastReceivedChequeKey(cheque.Chequebook), &lastReceivedCheque)
	if err != nil {
		if err != storage.ErrNotFound {
			return nil, err
		}

		// if this is the first cheque from this chequebook, verify with the factory.
		err = s.factory.VerifyChequebook(ctx, cheque.Chequebook)
		if err != nil {
			return nil, err
		}

		lastCumulativePayout = big.NewInt(0)
	} else {
		lastCumulativePayout = lastReceivedCheque.CumulativePayout
	}

	// check this cheque is actually increasing in value
	amount := big.NewInt(0).Sub(cheque.CumulativePayout, lastCumulativePayout)

	if amount.Cmp(big.NewInt(0)) <= 0 {
		return nil, ErrChequeNotIncreasing
	}

	deducedAmount := new(big.Int).Sub(amount, deduction)

	if deducedAmount.Cmp(exchange) < 0 {
		return nil, ErrChequeValueTooLow
	}

	// blockchain calls below
	contract := newChequebookContract(cheque.Chequebook, s.transactionService)

	// this does not change for the same chequebook
	expectedIssuer, err := contract.Issuer(ctx)
	if err != nil {
		return nil, err
	}

	// verify the cheque signature
	issuer, err := s.recoverChequeFunc(cheque, s.chaindID)
	if err != nil {
		return nil, err
	}

	if issuer != expectedIssuer {
		return nil, ErrChequeInvalid
	}

	// basic liquidity check
	// could be omitted as it is not particularly useful
	balance, err := contract.Balance(ctx)
	if err != nil {
		return nil, err
	}

	alreadyPaidOut, err := contract.PaidOut(ctx, s.beneficiary)
	if err != nil {
		return nil, err
	}

	if balance.Cmp(big.NewInt(0).Sub(cheque.CumulativePayout, alreadyPaidOut)) < 0 {
		return nil, ErrBouncingCheque
	}

	// store the accepted cheque
	err = s.store.Put(lastReceivedChequeKey(cheque.Chequebook), cheque)
	if err != nil {
		return nil, err
	}

	return amount, nil
}

// RecoverCheque recovers the issuer ethereum address from a signed cheque
func RecoverCheque(cheque *SignedCheque, chaindID int64) (common.Address, error) {
	eip712Data := eip712DataForCheque(&cheque.Cheque, chaindID)

	pubkey, err := crypto.RecoverEIP712(cheque.Signature, eip712Data)
	if err != nil {
		return common.Address{}, err
	}

	ethAddr, err := crypto.NewEthereumAddress(*pubkey)
	if err != nil {
		return common.Address{}, err
	}

	var issuer common.Address
	copy(issuer[:], ethAddr)
	return issuer, nil
}

// keyChequebook computes the chequebook a store entry is for.
func keyChequebook(key []byte, prefix string) (chequebook common.Address, err error) {
	k := string(key)

	split := strings.SplitAfter(k, prefix)
	if len(split) != 2 {
		return common.Address{}, errors.New("no peer in key")
	}
	return common.HexToAddress(split[1]), nil
}

// LastCheques returns the last received cheques from every known chequebook.
func (s *chequeStore) LastCheques() (map[common.Address]*SignedCheque, error) {
	result := make(map[common.Address]*SignedCheque)
	err := s.store.Iterate(lastReceivedChequePrefix, func(key, val []byte) (stop bool, err error) {
		addr, err := keyChequebook(key, lastReceivedChequePrefix+"_")
		if err != nil {
			return false, fmt.Errorf("parse address from key: %s: %w", string(key), err)
		}

		if _, ok := result[addr]; !ok {
			lastCheque, err := s.LastCheque(addr)
			if err != nil && err != ErrNoCheque {
				return false, err
			} else if err == ErrNoCheque {
				return false, nil
			}

			result[addr] = lastCheque
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
