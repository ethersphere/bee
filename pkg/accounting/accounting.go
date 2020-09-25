// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accounting

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/pricing"
	"github.com/ethersphere/bee/pkg/settlement"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	_              Interface = (*Accounting)(nil)
	balancesPrefix string    = "balance_"
)

// Interface is the Accounting interface.
type Interface interface {
	// Reserve reserves a portion of the balance for peer. Returns an error if
	// the operation risks exceeding the disconnect threshold.
	//
	// This should be called (always in combination with Release) before a
	// Credit action to prevent overspending in case of concurrent requests.
	Reserve(peer swarm.Address, price uint64) error
	// Release releases the reserved funds.
	Release(peer swarm.Address, price uint64)
	// Credit increases the balance the peer has with us (we "pay" the peer).
	Credit(peer swarm.Address, price uint64) error
	// Debit increases the balance we have with the peer (we get "paid" back).
	Debit(peer swarm.Address, price uint64) error
	// Balance returns the current balance for the given peer.
	Balance(peer swarm.Address) (int64, error)
	// Balances returns balances for all known peers.
	Balances() (map[string]int64, error)
}

// accountingPeer holds all in-memory accounting information for one peer.
type accountingPeer struct {
	lock             sync.Mutex // lock to be held during any accounting action for this peer
	reservedBalance  uint64     // amount currently reserved for active peer interaction
	paymentThreshold uint64     // the threshold at which the peer expects us to pay
}

// Options are options provided to Accounting.
type Options struct {
}

// Accounting is the main implementation of the accounting interface.
type Accounting struct {
	// Mutex for accessing the accountingPeers map.
	accountingPeersMu sync.Mutex
	accountingPeers   map[string]*accountingPeer
	logger            logging.Logger
	store             storage.StateStorer
	// The payment threshold in BZZ we communicate to our peers.
	paymentThreshold uint64
	// The amount in BZZ we let peers exceed the payment threshold before we
	// disconnect them.
	paymentTolerance uint64
	earlyPayment     uint64
	settlement       settlement.Interface
	pricing          pricing.Interface
	metrics          metrics
}

var (
	// ErrOverdraft denotes the expected debt in Reserve would exceed the payment thresholds.
	ErrOverdraft = errors.New("attempted overdraft")
	// ErrDisconnectThresholdExceeded denotes a peer has exceeded the disconnect threshold.
	ErrDisconnectThresholdExceeded = errors.New("disconnect threshold exceeded")
	// ErrInvalidPaymentTolerance denotes the payment tolerance is too high
	// compared to the payment threshold.
	ErrInvalidPaymentTolerance = errors.New("payment tolerance must be less than half the payment threshold")
	// ErrPeerNoBalance is the error returned if no balance in store exists for a peer
	ErrPeerNoBalance = errors.New("no balance for peer")
	// ErrOverflow denotes an arithmetic operation overflowed.
	ErrOverflow = errors.New("overflow error")
)

// NewAccounting creates a new Accounting instance with the provided options.
func NewAccounting(
	PaymentThreshold uint64,
	PaymentTolerance uint64,
	EarlyPayment uint64,
	Logger logging.Logger,
	Store storage.StateStorer,
	Settlement settlement.Interface,
	Pricing pricing.Interface,
	o Options,
) (*Accounting, error) {
	if PaymentTolerance+PaymentThreshold > math.MaxInt64 {
		return nil, fmt.Errorf("tolerance plus threshold too big: %w", ErrOverflow)
	}

	if PaymentTolerance > PaymentThreshold/2 {
		return nil, ErrInvalidPaymentTolerance
	}

	return &Accounting{
		accountingPeers:  make(map[string]*accountingPeer),
		paymentThreshold: PaymentThreshold,
		paymentTolerance: PaymentTolerance,
		earlyPayment:     EarlyPayment,
		logger:           Logger,
		store:            Store,
		settlement:       Settlement,
		pricing:          Pricing,
		metrics:          newMetrics(),
	}, nil
}

// Reserve reserves a portion of the balance for peer.
func (a *Accounting) Reserve(peer swarm.Address, price uint64) error {
	accountingPeer, err := a.getAccountingPeer(peer)
	if err != nil {
		return err
	}

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	currentBalance, err := a.Balance(peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			return fmt.Errorf("failed to load balance: %w", err)
		}
	}

	// Subtract already reserved amount from actual balance, to get expected balance
	expectedBalance, err := subtractI64mU64(currentBalance, accountingPeer.reservedBalance)
	if err != nil {
		return err
	}

	// Determine if we owe anything to the peer, if we owe less than 0, we conclude we owe nothing
	// This conversion is made safe by previous subtractI64mU64 not allowing MinInt64
	expectedDebt := -expectedBalance
	if expectedDebt < 0 {
		expectedDebt = 0
	}

	// Check if the expected debt is already over the payment threshold.
	if uint64(expectedDebt) > a.paymentThreshold {
		a.metrics.AccountingBlocksCount.Inc()
		return ErrOverdraft
	}

	// Check for safety of increase of reservedBalance by price
	if accountingPeer.reservedBalance+price < accountingPeer.reservedBalance {
		return ErrOverflow
	}

	accountingPeer.reservedBalance += price

	return nil
}

// Release releases reserved funds.
func (a *Accounting) Release(peer swarm.Address, price uint64) {
	accountingPeer, err := a.getAccountingPeer(peer)
	if err != nil {
		a.logger.Errorf("cannot release balance for peer: %v", err)
		return
	}

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	// NOTE: this should never happen if Reserve and Release calls are paired.
	if price > accountingPeer.reservedBalance {
		a.logger.Error("attempting to release more balance than was reserved for peer")
		accountingPeer.reservedBalance = 0
	} else {
		accountingPeer.reservedBalance -= price
	}
}

// Credit increases the amount of credit we have with the given peer
// (and decreases existing debt).
func (a *Accounting) Credit(peer swarm.Address, price uint64) error {
	accountingPeer, err := a.getAccountingPeer(peer)
	if err != nil {
		return err
	}

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	currentBalance, err := a.Balance(peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			return fmt.Errorf("failed to load balance: %w", err)
		}
	}

	// Calculate next balance by safely decreasing current balance with the price we credit
	nextBalance, err := subtractI64mU64(currentBalance, price)
	if err != nil {
		return err
	}

	a.logger.Tracef("crediting peer %v with price %d, new balance is %d", peer, price, nextBalance)

	// Get expectedbalance by safely decreasing current balance with reserved amounts
	expectedBalance, err := subtractI64mU64(currentBalance, accountingPeer.reservedBalance)
	if err != nil {
		return err
	}

	// Compute expected debt before update because reserve still includes the
	// amount that is deducted from the balance.
	// This conversion is made safe by previous subtractI64mU64 not allowing MinInt64
	expectedDebt := -expectedBalance
	if expectedDebt < 0 {
		expectedDebt = 0
	}

	err = a.store.Put(peerBalanceKey(peer), nextBalance)
	if err != nil {
		return fmt.Errorf("failed to persist balance: %w", err)
	}

	a.metrics.TotalCreditedAmount.Add(float64(price))
	a.metrics.CreditEventsCount.Inc()

	// If our expected debt is less than earlyPayment away from our payment threshold (which we assume is
	// also the peers payment threshold), trigger settlement.
	// we pay early to avoid needlessly blocking request later when concurrent requests occur and we are already close to the payment threshold
	threshold := accountingPeer.paymentThreshold
	if threshold > a.earlyPayment {
		threshold -= a.earlyPayment
	} else {
		threshold = 0
	}

	if uint64(expectedDebt) >= threshold {
		err = a.settle(peer, accountingPeer)
		if err != nil {
			a.logger.Errorf("failed to settle with peer %v: %v", peer, err)
		}
	}

	return nil
}

// Settle all debt with a peer. The lock on the accountingPeer must be held when
// called.
func (a *Accounting) settle(peer swarm.Address, balance *accountingPeer) error {
	oldBalance, err := a.Balance(peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			return fmt.Errorf("failed to load balance: %w", err)
		}
	}

	// Don't do anything if there is no actual debt.
	// This might be the case if the peer owes us and the total reserve for a
	// peer exceeds the payment treshold.
	if oldBalance >= 0 {
		return nil
	}

	// check safety of the following -1 * int64 conversion, all negative int64 have positive int64 equals except MinInt64
	if oldBalance == math.MinInt64 {
		return ErrOverflow
	}

	// This is safe because of the earlier check for oldbalance < 0 and the check for != MinInt64
	paymentAmount := uint64(-oldBalance)
	nextBalance := 0

	// Try to save the next balance first.
	// Otherwise we might pay and then not be able to save, forcing us to pay
	// again after restart.
	err = a.store.Put(peerBalanceKey(peer), nextBalance)
	if err != nil {
		return fmt.Errorf("failed to persist balance: %w", err)
	}

	err = a.settlement.Pay(context.Background(), peer, paymentAmount)
	if err != nil {
		err = fmt.Errorf("settlement for amount %d failed: %w", paymentAmount, err)
		// If the payment didn't succeed we should restore the old balance in
		// the state store.
		if storeErr := a.store.Put(peerBalanceKey(peer), oldBalance); storeErr != nil {
			a.logger.Errorf("failed to restore balance after failed settlement for peer %v: %v", peer, storeErr)
		}
		return err
	}

	return nil
}

// Debit increases the amount of debt we have with the given peer (and decreases
// existing credit).
func (a *Accounting) Debit(peer swarm.Address, price uint64) error {
	accountingPeer, err := a.getAccountingPeer(peer)
	if err != nil {
		return err
	}

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	currentBalance, err := a.Balance(peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			return fmt.Errorf("failed to load balance: %w", err)
		}
	}

	// Get nextBalance by safely increasing current balance with price
	nextBalance, err := addI64pU64(currentBalance, price)
	if err != nil {
		return err
	}

	a.logger.Tracef("debiting peer %v with price %d, new balance is %d", peer, price, nextBalance)

	err = a.store.Put(peerBalanceKey(peer), nextBalance)
	if err != nil {
		return fmt.Errorf("failed to persist balance: %w", err)
	}

	a.metrics.TotalDebitedAmount.Add(float64(price))
	a.metrics.DebitEventsCount.Inc()

	if nextBalance >= int64(a.paymentThreshold+a.paymentTolerance) {
		// peer too much in debt
		a.metrics.AccountingDisconnectsCount.Inc()
		return p2p.NewBlockPeerError(10000*time.Hour, ErrDisconnectThresholdExceeded)
	}

	return nil
}

// Balance returns the current balance for the given peer.
func (a *Accounting) Balance(peer swarm.Address) (balance int64, err error) {
	err = a.store.Get(peerBalanceKey(peer), &balance)

	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return 0, ErrPeerNoBalance
		}
		return 0, err
	}

	return balance, nil
}

// peerBalanceKey returns the balance storage key for the given peer.
func peerBalanceKey(peer swarm.Address) string {
	return fmt.Sprintf("%s%s", balancesPrefix, peer.String())
}

// getAccountingPeer returns the accountingPeer for a given swarm address.
// If not found in memory it will initialize it.
func (a *Accounting) getAccountingPeer(peer swarm.Address) (*accountingPeer, error) {
	a.accountingPeersMu.Lock()
	defer a.accountingPeersMu.Unlock()

	peerData, ok := a.accountingPeers[peer.String()]
	if !ok {
		peerData = &accountingPeer{
			reservedBalance: 0,
			// initially assume the peer has the same threshold as us
			paymentThreshold: a.paymentThreshold,
		}
		a.accountingPeers[peer.String()] = peerData
	}

	return peerData, nil
}

// Balances gets balances for all peers from store.
func (a *Accounting) Balances() (map[string]int64, error) {
	s := make(map[string]int64)

	err := a.store.Iterate(balancesPrefix, func(key, val []byte) (stop bool, err error) {
		addr, err := balanceKeyPeer(key)
		if err != nil {
			return false, fmt.Errorf("parse address from key: %s: %v", string(key), err)
		}

		if _, ok := s[addr.String()]; !ok {
			var storevalue int64
			err = a.store.Get(peerBalanceKey(addr), &storevalue)
			if err != nil {
				return false, fmt.Errorf("get peer %s balance: %v", addr.String(), err)
			}

			s[addr.String()] = storevalue
		}

		return false, nil
	})

	if err != nil {
		return nil, err
	}

	return s, nil
}

// balanceKeyPeer returns the embedded peer from the balance storage key.
func balanceKeyPeer(key []byte) (swarm.Address, error) {
	k := string(key)

	split := strings.SplitAfter(k, balancesPrefix)
	if len(split) != 2 {
		return swarm.ZeroAddress, errors.New("no peer in key")
	}

	addr, err := swarm.ParseHexAddress(split[1])
	if err != nil {
		return swarm.ZeroAddress, err
	}

	return addr, nil
}

// NotifyPayment implements the PaymentObserver interface. It is called by
// Settlement when we receive a payment.
func (a *Accounting) NotifyPayment(peer swarm.Address, amount uint64) error {
	accountingPeer, err := a.getAccountingPeer(peer)
	if err != nil {
		return err
	}

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	currentBalance, err := a.Balance(peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			return err
		}
	}

	nextBalance, err := subtractI64mU64(currentBalance, amount)
	if err != nil {
		return err
	}

	// Don't allow a payment to put use more into debt than the tolerance.
	// This is to prevent another node tricking us into settling by settling
	// first (e.g. send a bouncing cheque to trigger an honest cheque in swap).
	if nextBalance < -int64(a.paymentTolerance) {
		return fmt.Errorf("refusing to accept payment which would put us too much in debt, new balance would have been %d", nextBalance)
	}

	a.logger.Tracef("crediting peer %v with amount %d due to payment, new balance is %d", peer, amount, nextBalance)

	err = a.store.Put(peerBalanceKey(peer), nextBalance)
	if err != nil {
		return fmt.Errorf("failed to persist balance: %w", err)
	}

	return nil
}

// subtractI64mU64 is a helper function for safe subtraction of Int64 - Uint64
// It checks for
//   - overflow safety in conversion of uint64 to int64
//   - safety of the arithmetic
//   - whether ( -1 * result ) is still Int64, as MinInt64 in absolute sense is 1 larger than MaxInt64
// If result is MinInt64, we also return overflow error, for two reasons:
//   - in some cases we are going to use -1 * result in the following operations, which is secured by this check
//   - we also do not want to possibly store this value as balance, even if ( -1 * result ) is not used immediately afterwards, because it could
//		disable settleing for this amount as the value would create overflow
func subtractI64mU64(base int64, subtracted uint64) (result int64, err error) {
	if subtracted > math.MaxInt64 {
		return 0, ErrOverflow
	}

	result = base - int64(subtracted)
	if result > base {
		return 0, ErrOverflow
	}

	if result == math.MinInt64 {
		return 0, ErrOverflow
	}

	return result, nil
}

// addI64pU64 is a helper function for safe addition of Int64 + Uint64
// It checks for
//   - overflow safety in conversion of uint64 to int64
//   - safety of the arithmetic
func addI64pU64(a int64, b uint64) (result int64, err error) {
	if b > math.MaxInt64 {
		return 0, ErrOverflow
	}

	result = a + int64(b)
	if result < a {
		return 0, ErrOverflow
	}

	return result, nil
}

// NotifyPaymentThreshold should be called to notify accounting of changes in the payment threshold
func (a *Accounting) NotifyPaymentThreshold(peer swarm.Address, paymentThreshold uint64) error {
	accountingPeer, err := a.getAccountingPeer(peer)
	if err != nil {
		return err
	}

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	accountingPeer.paymentThreshold = paymentThreshold
	return nil
}
