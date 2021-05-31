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
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/settlement/swap/erc20"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/transaction"
	"github.com/ethersphere/go-sw3-abi/sw3abi"
)

// SendChequeFunc is a function to send cheques.
type SendChequeFunc func(cheque *SignedCheque) error

const (
	lastIssuedChequeKeyPrefix = "swap_chequebook_last_issued_cheque_"
	totalIssuedKey            = "swap_chequebook_total_issued_"
)

var (
	// ErrOutOfFunds is the error when the chequebook has not enough free funds for a cheque
	ErrOutOfFunds = errors.New("chequebook out of funds")
	// ErrInsufficientFunds is the error when the chequebook has not enough free funds for a user action
	ErrInsufficientFunds = errors.New("insufficient token balance")

	chequebookABI          = transaction.ParseABIUnchecked(sw3abi.ERC20SimpleSwapABIv0_3_1)
	chequeCashedEventType  = chequebookABI.Events["ChequeCashed"]
	chequeBouncedEventType = chequebookABI.Events["ChequeBounced"]
)

// Service is the main interface for interacting with the nodes chequebook.
type Service interface {
	// Deposit starts depositing erc20 token into the chequebook. This returns once the transactions has been broadcast.
	Deposit(ctx context.Context, amount *big.Int) (hash common.Hash, err error)
	// Withdraw starts withdrawing erc20 token from the chequebook. This returns once the transactions has been broadcast.
	Withdraw(ctx context.Context, amount *big.Int) (hash common.Hash, err error)
	// WaitForDeposit waits for the deposit transaction to confirm and verifies the result.
	WaitForDeposit(ctx context.Context, txHash common.Hash) error
	// Balance returns the token balance of the chequebook.
	Balance(ctx context.Context) (*big.Int, error)
	// AvailableBalance returns the token balance of the chequebook which is not yet used for uncashed cheques.
	AvailableBalance(ctx context.Context) (*big.Int, error)
	// Address returns the address of the used chequebook contract.
	Address() common.Address
	// Issue a new cheque for the beneficiary with an cumulativePayout amount higher than the last.
	Issue(ctx context.Context, beneficiary common.Address, amount *big.Int, sendChequeFunc SendChequeFunc) (*big.Int, error)
	// LastCheque returns the last cheque we issued for the beneficiary.
	LastCheque(beneficiary common.Address) (*SignedCheque, error)
	// LastCheque returns the last cheques for all beneficiaries.
	LastCheques() (map[common.Address]*SignedCheque, error)
}

type service struct {
	lock               sync.Mutex
	transactionService transaction.Service

	address      common.Address
	contract     *chequebookContract
	ownerAddress common.Address

	erc20Service erc20.Service

	store               storage.StateStorer
	chequeSigner        ChequeSigner
	totalIssuedReserved *big.Int
}

// New creates a new chequebook service for the provided chequebook contract.
func New(transactionService transaction.Service, address, ownerAddress common.Address, store storage.StateStorer, chequeSigner ChequeSigner, erc20Service erc20.Service) (Service, error) {
	return &service{
		transactionService:  transactionService,
		address:             address,
		contract:            newChequebookContract(address, transactionService),
		ownerAddress:        ownerAddress,
		erc20Service:        erc20Service,
		store:               store,
		chequeSigner:        chequeSigner,
		totalIssuedReserved: big.NewInt(0),
	}, nil
}

// Address returns the address of the used chequebook contract.
func (s *service) Address() common.Address {
	return s.address
}

// Deposit starts depositing erc20 token into the chequebook. This returns once the transactions has been broadcast.
func (s *service) Deposit(ctx context.Context, amount *big.Int) (hash common.Hash, err error) {
	balance, err := s.erc20Service.BalanceOf(ctx, s.ownerAddress)
	if err != nil {
		return common.Hash{}, err
	}

	// check we can afford this so we don't waste gas
	if balance.Cmp(amount) < 0 {
		return common.Hash{}, ErrInsufficientFunds
	}

	return s.erc20Service.Transfer(ctx, s.address, amount)
}

// Balance returns the token balance of the chequebook.
func (s *service) Balance(ctx context.Context) (*big.Int, error) {
	return s.contract.Balance(ctx)
}

// AvailableBalance returns the token balance of the chequebook which is not yet used for uncashed cheques.
func (s *service) AvailableBalance(ctx context.Context) (*big.Int, error) {
	totalIssued, err := s.totalIssued()
	if err != nil {
		return nil, err
	}

	balance, err := s.Balance(ctx)
	if err != nil {
		return nil, err
	}

	totalPaidOut, err := s.contract.TotalPaidOut(ctx)
	if err != nil {
		return nil, err
	}

	// balance plus totalPaidOut is the total amount ever put into the chequebook (ignoring deposits and withdrawals which cancelled out)
	// minus the total amount we issued from this chequebook this gives use the portion of the balance not covered by any cheques
	availableBalance := big.NewInt(0).Add(balance, totalPaidOut)
	availableBalance = availableBalance.Sub(availableBalance, totalIssued)
	return availableBalance, nil
}

// WaitForDeposit waits for the deposit transaction to confirm and verifies the result.
func (s *service) WaitForDeposit(ctx context.Context, txHash common.Hash) error {
	receipt, err := s.transactionService.WaitForReceipt(ctx, txHash)
	if err != nil {
		return err
	}
	if receipt.Status != 1 {
		return transaction.ErrTransactionReverted
	}
	return nil
}

// lastIssuedChequeKey computes the key where to store the last cheque for a beneficiary.
func lastIssuedChequeKey(beneficiary common.Address) string {
	return fmt.Sprintf("%s%x", lastIssuedChequeKeyPrefix, beneficiary)
}

func (s *service) reserveTotalIssued(ctx context.Context, amount *big.Int) (*big.Int, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	availableBalance, err := s.AvailableBalance(ctx)
	if err != nil {
		return nil, err
	}

	if amount.Cmp(big.NewInt(0).Sub(availableBalance, s.totalIssuedReserved)) > 0 {
		return nil, ErrOutOfFunds
	}

	s.totalIssuedReserved = s.totalIssuedReserved.Add(s.totalIssuedReserved, amount)
	return big.NewInt(0).Sub(availableBalance, amount), nil
}

func (s *service) unreserveTotalIssued(amount *big.Int) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.totalIssuedReserved = s.totalIssuedReserved.Sub(s.totalIssuedReserved, amount)
}

// Issue issues a new cheque and passes it to sendChequeFunc.
// The cheque is considered sent and saved when sendChequeFunc succeeds.
// The available balance which is available after sending the cheque is passed
// to the caller for it to be communicated over metrics.
func (s *service) Issue(ctx context.Context, beneficiary common.Address, amount *big.Int, sendChequeFunc SendChequeFunc) (*big.Int, error) {
	availableBalance, err := s.reserveTotalIssued(ctx, amount)
	if err != nil {
		return nil, err
	}
	defer s.unreserveTotalIssued(amount)

	var cumulativePayout *big.Int
	lastCheque, err := s.LastCheque(beneficiary)
	if err != nil {
		if err != ErrNoCheque {
			return nil, err
		}
		cumulativePayout = big.NewInt(0)
	} else {
		cumulativePayout = lastCheque.CumulativePayout
	}

	// increase cumulativePayout by amount
	cumulativePayout = cumulativePayout.Add(cumulativePayout, amount)

	// create and sign the new cheque
	cheque := Cheque{
		Chequebook:       s.address,
		CumulativePayout: cumulativePayout,
		Beneficiary:      beneficiary,
	}

	sig, err := s.chequeSigner.Sign(&Cheque{
		Chequebook:       s.address,
		CumulativePayout: cumulativePayout,
		Beneficiary:      beneficiary,
	})
	if err != nil {
		return nil, err
	}

	// actually send the check before saving to avoid double payment
	err = sendChequeFunc(&SignedCheque{
		Cheque:    cheque,
		Signature: sig,
	})
	if err != nil {
		return nil, err
	}

	err = s.store.Put(lastIssuedChequeKey(beneficiary), cheque)
	if err != nil {
		return nil, err
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	totalIssued, err := s.totalIssued()
	if err != nil {
		return nil, err
	}
	totalIssued = totalIssued.Add(totalIssued, amount)
	return availableBalance, s.store.Put(totalIssuedKey, totalIssued)
}

// returns the total amount in cheques issued so far
func (s *service) totalIssued() (totalIssued *big.Int, err error) {
	err = s.store.Get(totalIssuedKey, &totalIssued)
	if err != nil {
		if err != storage.ErrNotFound {
			return nil, err
		}
		return big.NewInt(0), nil
	}
	return totalIssued, nil
}

// LastCheque returns the last cheque we issued for the beneficiary.
func (s *service) LastCheque(beneficiary common.Address) (*SignedCheque, error) {
	var lastCheque *SignedCheque
	err := s.store.Get(lastIssuedChequeKey(beneficiary), &lastCheque)
	if err != nil {
		if err != storage.ErrNotFound {
			return nil, err
		}
		return nil, ErrNoCheque
	}
	return lastCheque, nil
}

func keyBeneficiary(key []byte, prefix string) (beneficiary common.Address, err error) {
	k := string(key)

	split := strings.SplitAfter(k, prefix)
	if len(split) != 2 {
		return common.Address{}, errors.New("no beneficiary in key")
	}
	return common.HexToAddress(split[1]), nil
}

// LastCheque returns the last cheques for all beneficiaries.
func (s *service) LastCheques() (map[common.Address]*SignedCheque, error) {
	result := make(map[common.Address]*SignedCheque)
	err := s.store.Iterate(lastIssuedChequeKeyPrefix, func(key, val []byte) (stop bool, err error) {
		addr, err := keyBeneficiary(key, lastIssuedChequeKeyPrefix)
		if err != nil {
			return false, fmt.Errorf("parse address from key: %s: %w", string(key), err)
		}

		if _, ok := result[addr]; !ok {

			lastCheque, err := s.LastCheque(addr)
			if err != nil {
				return false, err
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

func (s *service) Withdraw(ctx context.Context, amount *big.Int) (hash common.Hash, err error) {
	availableBalance, err := s.AvailableBalance(ctx)
	if err != nil {
		return common.Hash{}, err
	}

	// check we can afford this so we don't waste gas and don't risk bouncing cheques
	if availableBalance.Cmp(amount) < 0 {
		return common.Hash{}, ErrInsufficientFunds
	}

	callData, err := chequebookABI.Pack("withdraw", amount)
	if err != nil {
		return common.Hash{}, err
	}

	request := &transaction.TxRequest{
		To:       &s.address,
		Data:     callData,
		GasPrice: sctx.GetGasPrice(ctx),
		GasLimit: 0,
		Value:    big.NewInt(0),
	}

	txHash, err := s.transactionService.Send(ctx, request)
	if err != nil {
		return common.Hash{}, err
	}

	return txHash, nil
}
