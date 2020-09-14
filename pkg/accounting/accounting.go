// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accounting

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/settlement"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	_              Interface = (*Accounting)(nil)
	balancesPrefix string    = "balance_"
)

// Interface is the main interface for Accounting
type Interface interface {
	// Reserve reserves a portion of the balance for peer
	// It returns an error if the operation would risk exceeding the disconnect threshold
	// This should be called (always in combination with Release) before a Credit action to prevent overspending in case of concurrent requests
	Reserve(peer swarm.Address, price uint64) error
	// Release releases reserved funds
	Release(peer swarm.Address, price uint64)
	// Credit increases the balance the peer has with us (we "pay" the peer)
	Credit(peer swarm.Address, price uint64) error
	// Debit increases the balance we have with the peer (we get "paid")
	Debit(peer swarm.Address, price uint64) error
	// Balance returns the current balance for the given peer
	Balance(peer swarm.Address) (int64, error)
	// Balances returns balances for all known peers
	Balances() (map[string]int64, error)
}

// accountingPeer holds all in-memory accounting information for one peer
type accountingPeer struct {
	lock            sync.Mutex // lock to be held during any accounting action for this peer
	reservedBalance uint64     // amount currently reserved for active peer interaction
}

// Options for accounting
type Options struct {
	PaymentThreshold uint64
	PaymentTolerance uint64
	Logger           logging.Logger
	Store            storage.StateStorer
	Settlement       settlement.Interface
}

// Accounting is the main implementation of the accounting interface
type Accounting struct {
	accountingPeersMu sync.Mutex // mutex for accessing the accountingPeers map
	accountingPeers   map[string]*accountingPeer
	logger            logging.Logger
	store             storage.StateStorer
	paymentThreshold  uint64 // the payment threshold in BZZ we communicate to our peers
	paymentTolerance  uint64 // the amount in BZZ we let peers exceed the payment threshold before disconnected
	settlement        settlement.Interface
	metrics           metrics
}

var (
	// ErrOverdraft is the error returned if the expected debt in Reserve would exceed the payment thresholds
	ErrOverdraft = errors.New("attempted overdraft")
	// ErrDisconnectThresholdExceeded is the error returned if a peer has exceeded the disconnect threshold
	ErrDisconnectThresholdExceeded = errors.New("disconnect threshold exceeded")
	// ErrInvalidPaymentTolerance is the error returned if the payment tolerance is too high compared to the payment threshold
	ErrInvalidPaymentTolerance = errors.New("payment tolerance must be less than half the payment threshold")
	// ErrPeerNoBalance is the error returned if no balance in store exists for a peer
	ErrPeerNoBalance = errors.New("no balance for peer")
)

// NewAccounting creates a new Accounting instance with the provided options
func NewAccounting(o Options) (*Accounting, error) {
	if o.PaymentTolerance > o.PaymentThreshold/2 {
		return nil, ErrInvalidPaymentTolerance
	}

	return &Accounting{
		accountingPeers:  make(map[string]*accountingPeer),
		paymentThreshold: o.PaymentThreshold,
		paymentTolerance: o.PaymentTolerance,
		logger:           o.Logger,
		store:            o.Store,
		settlement:       o.Settlement,
		metrics:          newMetrics(),
	}, nil
}

// Reserve reserves a portion of the balance for peer
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

	expectedDebt := -(currentBalance - int64(accountingPeer.reservedBalance))
	if expectedDebt < 0 {
		expectedDebt = 0
	}

	// check if the expected debt is already over the payment threshold
	if uint64(expectedDebt) > a.paymentThreshold {
		a.metrics.AccountingBlocksCount.Inc()
		return ErrOverdraft
	}

	accountingPeer.reservedBalance += price
	return nil
}

// Release releases reserved funds
func (a *Accounting) Release(peer swarm.Address, price uint64) {
	accountingPeer, err := a.getAccountingPeer(peer)
	if err != nil {
		a.logger.Errorf("cannot release balance for peer: %v", err)
		return
	}

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	if price > accountingPeer.reservedBalance {
		// If Reserve and Release calls are always paired this should never happen
		a.logger.Error("attempting to release more balance than was reserved for peer")
		accountingPeer.reservedBalance = 0
	} else {
		accountingPeer.reservedBalance -= price
	}
}

// Credit increases the amount of credit we have with the given peer (and decreases existing debt).
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

	nextBalance := currentBalance - int64(price)

	a.logger.Tracef("crediting peer %v with price %d, new balance is %d", peer, price, nextBalance)

	// compute expected debt before update because reserve still includes the amount that is deducted from the balance
	expectedDebt := -(currentBalance - int64(accountingPeer.reservedBalance))
	if expectedDebt < 0 {
		expectedDebt = 0
	}

	err = a.store.Put(peerBalanceKey(peer), nextBalance)
	if err != nil {
		return fmt.Errorf("failed to persist balance: %w", err)
	}

	a.metrics.TotalCreditedAmount.Add(float64(price))
	a.metrics.CreditEventsCount.Inc()

	// if our expected debt exceeds our payment threshold (which we assume is also the peers payment threshold), trigger settlement
	if uint64(expectedDebt) >= a.paymentThreshold {
		err = a.settle(peer, accountingPeer)
		if err != nil {
			a.logger.Errorf("failed to settle with peer %v: %v", peer, err)
		}
	}

	return nil
}

// settle all debt with a peer
// the lock on the accountingPeer must be held when called
func (a *Accounting) settle(peer swarm.Address, balance *accountingPeer) error {
	oldBalance, err := a.Balance(peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			return fmt.Errorf("failed to load balance: %w", err)
		}
	}

	// don't do anything if there is no actual debt
	// this might be the case if the peer owes us and the total reserve for a peer exceeds the payment treshhold
	if oldBalance >= 0 {
		return nil
	}

	paymentAmount := uint64(-oldBalance)
	nextBalance := oldBalance + int64(paymentAmount)

	// try to save the next balance first
	// otherwise we might pay and then not be able to save, thus paying again after restart
	err = a.store.Put(peerBalanceKey(peer), nextBalance)
	if err != nil {
		return fmt.Errorf("failed to persist balance: %w", err)
	}

	err = a.settlement.Pay(context.Background(), peer, paymentAmount)
	if err != nil {
		err = fmt.Errorf("settlement for amount %d failed: %w", paymentAmount, err)
		// if the payment didn't work we should restore the old balance in the state store
		if storeErr := a.store.Put(peerBalanceKey(peer), nextBalance); storeErr != nil {
			a.logger.Errorf("failed to restore balance after failed settlement for peer %v: %v", peer, storeErr)
		}
		return err
	}
	return nil
}

// Debit increases the amount of debt we have with the given peer (and decreases existing credit)
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
	nextBalance := currentBalance + int64(price)

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

// Balance returns the current balance for the given peer
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

// get the balance storage key for the given peer
func peerBalanceKey(peer swarm.Address) string {
	return fmt.Sprintf("%s%s", balancesPrefix, peer.String())
}

// getAccountingPeer gets the accountingPeer for a given swarm address
// If not in memory it will initialize it
func (a *Accounting) getAccountingPeer(peer swarm.Address) (*accountingPeer, error) {
	a.accountingPeersMu.Lock()
	defer a.accountingPeersMu.Unlock()

	peerData, ok := a.accountingPeers[peer.String()]
	if !ok {
		peerData = &accountingPeer{
			reservedBalance: 0,
		}
		a.accountingPeers[peer.String()] = peerData
	}

	return peerData, nil
}

// Balances gets balances for all peers from store
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

// get the embedded peer from the balance storage key
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

// NotifyPayment is called by Settlement when we received payment
// Implements the PaymentObserver interface
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
	nextBalance := currentBalance - int64(amount)

	// don't allow a payment to put use more into debt than the tolerance
	// this is to prevent another node tricking us into settling by settling first (e.g. send a bouncing cheque to trigger an honest cheque in swap)
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
