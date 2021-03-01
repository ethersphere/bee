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

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/sw3-bindings/v3/simpleswapfactory"
)

var (
	// ErrNoCashout is the error if there has not been any cashout action for the chequebook
	ErrNoCashout = errors.New("no prior cashout")
)

// CashoutService is the service responsible for managing cashout actions
type CashoutService interface {
	// CashCheque sends a cashing transaction for the last cheque of the chequebook
	CashCheque(ctx context.Context, chequebook common.Address, recipient common.Address) (common.Hash, error)
	// CashoutStatus gets the status of the latest cashout transaction for the chequebook
	CashoutStatus(ctx context.Context, chequebookAddress common.Address) (*CashoutStatus, error)
}

type cashoutService struct {
	store                 storage.StateStorer
	simpleSwapBindingFunc SimpleSwapBindingFunc
	backend               transaction.Backend
	transactionService    transaction.Service
	chequebookABI         abi.ABI
	chequeStore           ChequeStore
}

// CashoutStatus is the action plus its result
type CashoutStatus struct {
	TxHash   common.Hash
	Cheque   SignedCheque // the cheque that was used to cashout which may be different from the latest cheque
	Result   *CashChequeResult
	Reverted bool
}

// CashChequeResult summarizes the result of a CashCheque or CashChequeBeneficiary call
type CashChequeResult struct {
	Beneficiary      common.Address // beneficiary of the cheque
	Recipient        common.Address // address which received the funds
	Caller           common.Address // caller of cashCheque
	TotalPayout      *big.Int       // total amount that was paid out in this call
	CumulativePayout *big.Int       // cumulative payout of the cheque that was cashed
	CallerPayout     *big.Int       // payout for the caller of cashCheque
	Bounced          bool           // indicates wether parts of the cheque bounced
}

// cashoutAction is the data we store for a cashout
type cashoutAction struct {
	TxHash common.Hash
	Cheque SignedCheque // the cheque that was used to cashout which may be different from the latest cheque
}

// NewCashoutService creates a new CashoutService
func NewCashoutService(
	store storage.StateStorer,
	simpleSwapBindingFunc SimpleSwapBindingFunc,
	backend transaction.Backend,
	transactionService transaction.Service,
	chequeStore ChequeStore,
) (CashoutService, error) {
	chequebookABI, err := abi.JSON(strings.NewReader(simpleswapfactory.ERC20SimpleSwapABI))
	if err != nil {
		return nil, err
	}

	return &cashoutService{
		store:                 store,
		simpleSwapBindingFunc: simpleSwapBindingFunc,
		backend:               backend,
		transactionService:    transactionService,
		chequebookABI:         chequebookABI,
		chequeStore:           chequeStore,
	}, nil
}

// cashoutActionKey computes the store key for the last cashout action for the chequebook
func cashoutActionKey(chequebook common.Address) string {
	return fmt.Sprintf("swap_cashout_%x", chequebook)
}

// CashCheque sends a cashout transaction for the last cheque of the chequebook
func (s *cashoutService) CashCheque(ctx context.Context, chequebook, recipient common.Address) (common.Hash, error) {
	cheque, err := s.chequeStore.LastCheque(chequebook)
	if err != nil {
		return common.Hash{}, err
	}

	callData, err := s.chequebookABI.Pack("cashChequeBeneficiary", recipient, cheque.CumulativePayout, cheque.Signature)
	if err != nil {
		return common.Hash{}, err
	}

	request := &transaction.TxRequest{
		To:       &chequebook,
		Data:     callData,
		GasPrice: nil,
		GasLimit: 0,
		Value:    big.NewInt(0),
	}

	txHash, err := s.transactionService.Send(ctx, request)
	if err != nil {
		return common.Hash{}, err
	}

	err = s.store.Put(cashoutActionKey(chequebook), &cashoutAction{
		TxHash: txHash,
		Cheque: *cheque,
	})
	if err != nil {
		return common.Hash{}, err
	}

	return txHash, nil
}

// CashoutStatus gets the status of the latest cashout transaction for the chequebook
func (s *cashoutService) CashoutStatus(ctx context.Context, chequebookAddress common.Address) (*CashoutStatus, error) {
	var action *cashoutAction
	err := s.store.Get(cashoutActionKey(chequebookAddress), &action)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrNoCashout
		}
		return nil, err
	}

	_, pending, err := s.backend.TransactionByHash(ctx, action.TxHash)
	if err != nil {
		return nil, err
	}

	if pending {
		return &CashoutStatus{
			TxHash:   action.TxHash,
			Cheque:   action.Cheque,
			Result:   nil,
			Reverted: false,
		}, nil
	}

	receipt, err := s.backend.TransactionReceipt(ctx, action.TxHash)
	if err != nil {
		return nil, err
	}

	if receipt.Status == types.ReceiptStatusFailed {
		return &CashoutStatus{
			TxHash:   action.TxHash,
			Cheque:   action.Cheque,
			Result:   nil,
			Reverted: true,
		}, nil
	}

	result, err := s.parseCashChequeBeneficiaryReceipt(chequebookAddress, receipt)
	if err != nil {
		return nil, err
	}

	return &CashoutStatus{
		TxHash:   action.TxHash,
		Cheque:   action.Cheque,
		Result:   result,
		Reverted: false,
	}, nil
}

// parseCashChequeBeneficiaryReceipt processes the receipt from a CashChequeBeneficiary transaction
func (s *cashoutService) parseCashChequeBeneficiaryReceipt(chequebookAddress common.Address, receipt *types.Receipt) (*CashChequeResult, error) {
	result := &CashChequeResult{
		Bounced: false,
	}

	binding, err := s.simpleSwapBindingFunc(chequebookAddress, s.backend)
	if err != nil {
		return nil, err
	}

	for _, log := range receipt.Logs {
		if log.Address != chequebookAddress {
			continue
		}
		if event, err := binding.ParseChequeCashed(*log); err == nil {
			result.Beneficiary = event.Beneficiary
			result.Caller = event.Caller
			result.CallerPayout = event.CallerPayout
			result.TotalPayout = event.TotalPayout
			result.CumulativePayout = event.CumulativePayout
			result.Recipient = event.Recipient
		} else if _, err := binding.ParseChequeBounced(*log); err == nil {
			result.Bounced = true
		}
	}

	return result, nil
}

// Equal compares to CashChequeResults
func (r *CashChequeResult) Equal(o *CashChequeResult) bool {
	if r.Beneficiary != o.Beneficiary {
		return false
	}
	if r.Bounced != o.Bounced {
		return false
	}
	if r.Caller != o.Caller {
		return false
	}
	if r.CallerPayout.Cmp(o.CallerPayout) != 0 {
		return false
	}
	if r.CumulativePayout.Cmp(o.CumulativePayout) != 0 {
		return false
	}
	if r.Recipient != o.Recipient {
		return false
	}
	if r.TotalPayout.Cmp(o.TotalPayout) != 0 {
		return false
	}
	return true
}
