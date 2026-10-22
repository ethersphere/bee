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
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/sctx"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/erc20"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/util/abiutil"
	"github.com/ethersphere/go-sw3-abi/sw3abi"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "chequebook"

// SendChequeFunc is a function to send cheques.
type SendChequeFunc func(cheque *SignedCheque) error

const (
	lastIssuedChequeKeyPrefix = "swap_chequebook_last_issued_cheque_"
	totalIssuedKey            = "swap_chequebook_total_issued_"

	coveringBalanceCacheValidity = 5 * time.Minute
	coveringBalanceReadTimeout   = 5 * time.Second
	minedWithdrawalBlocks        = 5
)

var (
	// ErrOutOfFunds is the error when the chequebook has not enough free funds for a cheque
	ErrOutOfFunds = errors.New("chequebook out of funds")
	// ErrInsufficientFunds is the error when the chequebook has not enough free funds for a user action
	ErrInsufficientFunds = errors.New("insufficient token balance")

	chequebookABI          = abiutil.MustParseABI(sw3abi.ERC20SimpleSwapABIv0_6_9)
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
	// LastCheques returns the last cheques for all beneficiaries.
	LastCheques() (map[common.Address]*SignedCheque, error)
}

type service struct {
	lock               sync.Mutex
	logger             log.Logger
	transactionService transaction.Service

	address      common.Address
	contract     *chequebookContract
	ownerAddress common.Address

	erc20Service erc20.Service

	store                      storage.StateStorer
	chequeSigner               ChequeSigner
	totalIssuedReserved        *big.Int
	coveringBalance            *big.Int
	coveringBalanceAt          time.Time
	coveringBalanceReadTimeout time.Duration
	withdrawalReceiptHold      time.Duration
	withdrawalsReserved        *big.Int
	pendingWithdrawals         map[common.Hash]*big.Int
	metrics                    metrics
}

// New creates a new chequebook service for the provided chequebook contract.
func New(transactionService transaction.Service, address, ownerAddress common.Address, store storage.StateStorer, chequeSigner ChequeSigner, erc20Service erc20.Service, blockTime time.Duration) (Service, error) {
	return &service{
		logger:                     log.NewLogger(loggerName).Register(),
		transactionService:         transactionService,
		address:                    address,
		contract:                   newChequebookContract(address, transactionService),
		ownerAddress:               ownerAddress,
		erc20Service:               erc20Service,
		store:                      store,
		chequeSigner:               chequeSigner,
		totalIssuedReserved:        big.NewInt(0),
		coveringBalanceReadTimeout: coveringBalanceReadTimeout,
		withdrawalReceiptHold:      minedWithdrawalBlocks * blockTime,
		withdrawalsReserved:        big.NewInt(0),
		pendingWithdrawals:         make(map[common.Hash]*big.Int),
		metrics:                    newMetrics(),
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
	s.lock.Lock()
	defer s.lock.Unlock()

	available, _, err := s.availableBalance(ctx, true)
	return available, err
}

func (s *service) availableBalance(ctx context.Context, refresh bool) (*big.Int, bool, error) {
	totalIssued, err := s.totalIssued()
	if err != nil {
		return nil, false, err
	}

	covering, fromCache, err := s.getOrLoadCoveringBalance(ctx, refresh)
	if err != nil {
		return nil, false, err
	}

	available := new(big.Int).Sub(covering, totalIssued)
	available.Sub(available, s.withdrawalsReserved)
	for _, pending := range s.pendingWithdrawals {
		available.Sub(available, pending)
	}

	return available, fromCache, nil
}

// When the cached figure cannot cover amount, the balance is reloaded from chain first,
// because a deposit or an external top-up might not be in the cache.
func (s *service) availableBalanceFor(ctx context.Context, amount *big.Int) (*big.Int, error) {
	available, fromCache, err := s.availableBalance(ctx, false)
	if err != nil {
		return nil, err
	}
	if fromCache && !s.covers(available, amount) {
		available, _, err = s.availableBalance(ctx, true)
		if err != nil {
			return nil, err
		}
	}
	return available, nil
}

// covers reports whether amount still fits after cheques that are being issued.
func (s *service) covers(available, amount *big.Int) bool {
	return amount.Cmp(new(big.Int).Sub(available, s.totalIssuedReserved)) <= 0
}

func (s *service) coveringBalanceValid() bool {
	return s.coveringBalance != nil && time.Since(s.coveringBalanceAt) < coveringBalanceCacheValidity
}

// getOrLoadCoveringBalance returns balance plus totalPaidOut.
// forceRefresh always reads the chain. Otherwise the cached value is returned
// while it is younger than coveringBalanceCacheValidity. The bool is true when
// the returned value came from the cache.
func (s *service) getOrLoadCoveringBalance(ctx context.Context, forceRefresh bool) (*big.Int, bool, error) {
	if !forceRefresh {
		if s.coveringBalanceValid() {
			s.metrics.CoveringBalanceCacheHit.Inc()
			return new(big.Int).Set(s.coveringBalance), true, nil
		}
		s.metrics.CoveringBalanceCacheMiss.Inc()
	}

	coveringBalance, err := s.getCoveringBalance(ctx)
	if err != nil {
		s.coveringBalance = nil
		s.coveringBalanceAt = time.Time{}
		return nil, false, err
	}

	s.coveringBalance = coveringBalance
	s.coveringBalanceAt = time.Now()
	setTokenGauge(s.metrics.CoveringBalance, s.coveringBalance)
	return new(big.Int).Set(s.coveringBalance), false, nil
}

// getCoveringBalance is called with s.lock held, so a stalled RPC must not
// outlive the timeout or it would block every cheque issue.
func (s *service) getCoveringBalance(ctx context.Context) (*big.Int, error) {
	ctx, cancel := context.WithTimeout(ctx, s.coveringBalanceReadTimeout)
	defer cancel()

	totalPaidOut, err := s.contract.TotalPaidOut(ctx)
	if err != nil {
		return nil, err
	}
	balance, err := s.Balance(ctx)
	if err != nil {
		return nil, err
	}
	return new(big.Int).Add(balance, totalPaidOut), nil
}

func (s *service) invalidateCoveringBalance() {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.metrics.CoveringBalanceCacheInvalidation.Inc()
	s.coveringBalance = nil
	s.coveringBalanceAt = time.Time{}
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

	s.invalidateCoveringBalance()
	return nil
}

// lastIssuedChequeKey computes the key where to store the last cheque for a beneficiary.
func lastIssuedChequeKey(beneficiary common.Address) string {
	return fmt.Sprintf("%s%x", lastIssuedChequeKeyPrefix, beneficiary)
}

func (s *service) reserveTotalIssued(ctx context.Context, amount *big.Int) (*big.Int, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	availableBalance, err := s.availableBalanceFor(ctx, amount)
	if err != nil {
		return nil, err
	}

	if !s.covers(availableBalance, amount) {
		s.metrics.OutOfFunds.Inc()
		setTokenGauge(s.metrics.AvailableBalance, new(big.Int).Sub(availableBalance, s.totalIssuedReserved))
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
		if !errors.Is(err, ErrNoCheque) {
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
	if err := s.store.Put(totalIssuedKey, totalIssued); err != nil {
		return nil, err
	}
	setTokenGauge(s.metrics.AvailableBalance, availableBalance)
	return availableBalance, nil
}

// returns the total amount in cheques issued so far
func (s *service) totalIssued() (totalIssued *big.Int, err error) {
	err = s.store.Get(totalIssuedKey, &totalIssued)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
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
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
		return nil, ErrNoCheque
	}

	if lastCheque == nil {
		return nil, fmt.Errorf("nil cheque loaded from statestore for beneficiary %x: %w", beneficiary, ErrNoCheque)
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

// LastCheques returns the last cheques for all beneficiaries.
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
				if errors.Is(err, ErrNoCheque) {
					return false, nil
				}

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
	amount = new(big.Int).Set(amount)

	callData, err := chequebookABI.Pack("withdraw", amount)
	if err != nil {
		return common.Hash{}, err
	}

	if err := s.reserveWithdrawal(ctx, amount); err != nil {
		return common.Hash{}, err
	}

	request := &transaction.TxRequest{
		To:          &s.address,
		Data:        callData,
		GasPrice:    sctx.GetGasPrice(ctx),
		GasLimit:    95000,
		Value:       big.NewInt(0),
		Description: fmt.Sprintf("chequebook withdrawal of %d BZZ", amount),
	}

	txHash, err := s.transactionService.Send(ctx, request, transaction.DefaultTipBoostPercent)

	s.lock.Lock()
	s.withdrawalsReserved.Sub(s.withdrawalsReserved, amount)
	if err == nil {
		s.pendingWithdrawals[txHash] = amount
	}
	s.lock.Unlock()

	if err != nil {
		return common.Hash{}, err
	}

	go s.waitForWithdrawal(txHash, amount)
	return txHash, nil
}

func (s *service) reserveWithdrawal(ctx context.Context, amount *big.Int) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	availableBalance, err := s.availableBalanceFor(ctx, amount)
	if err != nil {
		return err
	}

	// check we can afford this so we don't waste gas and don't risk bouncing cheques
	if !s.covers(availableBalance, amount) {
		return ErrInsufficientFunds
	}

	s.withdrawalsReserved.Add(s.withdrawalsReserved, amount)
	return nil
}

// waitForWithdrawal keeps a mined withdrawal deducted for 5 blocks, then drops it and reloads the covering balance from chain.
// A reverted or cancelled withdrawal is dropped immediately.
// On monitor shutdown the withdrawal stays deducted for the rest of the process lifetime.
func (s *service) waitForWithdrawal(txHash common.Hash, amount *big.Int) {
	receipt, err := s.transactionService.WaitForReceipt(context.Background(), txHash)
	if err != nil {
		if errors.Is(err, transaction.ErrTransactionCancelled) {
			s.logger.Warning("withdrawal transaction was not mined", "tx", txHash, "amount", amount, "error", err)
			s.lock.Lock()
			delete(s.pendingWithdrawals, txHash)
			s.lock.Unlock()
			return
		}
		if !errors.Is(err, transaction.ErrMonitorClosed) {
			// The transaction is already broadcast. Keep it deducted; dropping it would invite a second withdrawal.
			s.logger.Error(err, "watching withdrawal transaction failed", "tx", txHash, "amount", amount)
		}
		return
	}

	s.logger.Info("withdrawal receipt received", "tx", txHash, "amount", amount, "status", receipt.Status, "block", receipt.BlockNumber)
	if receipt.Status != 1 {
		s.lock.Lock()
		delete(s.pendingWithdrawals, txHash)
		s.lock.Unlock()
		return
	}
	// The withdrawal is already mined, but the next balance read can hit an RPC that has not seen that block yet
	time.Sleep(s.withdrawalReceiptHold)
	s.releaseMinedWithdrawal(txHash)
}

func (s *service) releaseMinedWithdrawal(txHash common.Hash) {
	s.lock.Lock()
	defer s.lock.Unlock()

	_, _, err := s.getOrLoadCoveringBalance(context.Background(), true)
	if err != nil {
		s.logger.Error(err, "reload covering balance after withdrawal failed")
	} else {
		setTokenGauge(s.metrics.CoveringBalance, s.coveringBalance)
	}
	delete(s.pendingWithdrawals, txHash)
}
