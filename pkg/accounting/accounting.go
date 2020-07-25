// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accounting

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/settlement"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var _ Interface = (*Accounting)(nil)

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
}

// PeerBalance holds all relevant accounting information for one peer
type PeerBalance struct {
	lock     sync.Mutex
	balance  int64  // amount that the peer owes us if positive, our debt if negative
	reserved uint64 // amount currently reserved for active peer interaction
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
	balancesMu       sync.Mutex // mutex for accessing the balances map
	balances         map[string]*PeerBalance
	logger           logging.Logger
	store            storage.StateStorer
	paymentThreshold uint64 // the debt threshold we communicate to our peers
	paymentTolerance uint64 // the amount we let peers exceed the payment threshold before disconnected
	settlement       settlement.Interface
}

var (
	ErrOverdraft = errors.New("attempted overdraft")
)

// NewAccounting creates a new Accounting instance with the provided options
func NewAccounting(o Options) *Accounting {
	return &Accounting{
		balances:         make(map[string]*PeerBalance),
		paymentThreshold: o.PaymentThreshold,
		paymentTolerance: o.PaymentTolerance,
		logger:           o.Logger,
		store:            o.Store,
		settlement:       o.Settlement,
	}
}

// Reserve reserves a portion of the balance for peer
func (a *Accounting) Reserve(peer swarm.Address, price uint64) error {
	balance, err := a.getPeerBalance(peer)
	if err != nil {
		return err
	}

	balance.lock.Lock()
	defer balance.lock.Unlock()

	// check if the previously reserved balance is already over the payment thresholds
	// the disconnectThreshold is stored as a positive value which is why it must be negated prior to comparison
	if balance.expectedDebt() > a.paymentThreshold {
		return fmt.Errorf("%w with peer %v", ErrOverdraft, peer)
	}

	// since we pay this we have to reduce this (positive quantity) from the balance
	balance.reserved += price

	return nil
}

// Release releases reserved funds
func (a *Accounting) Release(peer swarm.Address, price uint64) {
	balance, err := a.getPeerBalance(peer)
	if err != nil {
		a.logger.Errorf("cannot release balance for peer: %v", err)
		return
	}

	balance.lock.Lock()
	defer balance.lock.Unlock()

	if price > balance.reserved {
		// If Reserve and Release calls are always paired this should never happen
		a.logger.Error("attempting to release more balance than was reserved for peer")
		balance.reserved = 0
	} else {
		balance.reserved -= price
	}
}

// Credit increases the amount of credit we have with the given peer (and decreases existing debt).
func (a *Accounting) Credit(peer swarm.Address, price uint64) error {
	balance, err := a.getPeerBalance(peer)
	if err != nil {
		return err
	}

	balance.lock.Lock()
	defer balance.lock.Unlock()

	nextBalance := balance.balance - int64(price)

	a.logger.Tracef("crediting peer %v with price %d, new balance is %d", peer, price, nextBalance)

	err = a.store.Put(balanceKey(peer), nextBalance)
	if err != nil {
		return err
	}

	balance.balance = nextBalance

	// if we are (including reservations) above our payment threshold (which we assume is also the peers payment threshold), trigger settlement
	if balance.expectedDebt() > a.paymentThreshold {
		paymentAmount := uint64(-balance.balance)
		err = a.settlement.Pay(peer, paymentAmount)
		if err != nil {
			a.logger.Errorf("payment for peer %v with amount %d failed: %v", peer, paymentAmount, err)
			return nil
		}

		balance.balance += int64(paymentAmount)
		err = a.store.Put(balanceKey(peer), balance.balance)
		if err != nil {
			a.logger.Errorf("failed to persist balance for peer %v: %w", peer, err)
			return nil
		}
	}

	return nil
}

// Debit increases the amount of debt we have with the given peer (and decreases existing credit)
func (a *Accounting) Debit(peer swarm.Address, price uint64) error {
	balance, err := a.getPeerBalance(peer)
	if err != nil {
		return err
	}

	balance.lock.Lock()
	defer balance.lock.Unlock()

	nextBalance := balance.balance + int64(price)

	a.logger.Tracef("debiting peer %v with price %d, new balance is %d", peer, price, nextBalance)

	err = a.store.Put(balanceKey(peer), nextBalance)
	if err != nil {
		return fmt.Errorf("failed to persist balance for peer %v: %w", peer, err)
	}

	balance.balance = nextBalance

	if nextBalance >= int64(a.paymentThreshold+a.paymentTolerance) {
		// peer to much in debt
		return p2p.NewDisconnectError(fmt.Errorf("disconnect threshold exceeded for peer %s", peer.String()))
	}

	return nil
}

// Balance returns the current balance for the given peer
func (a *Accounting) Balance(peer swarm.Address) (int64, error) {
	peerBalance, err := a.getPeerBalance(peer)
	if err != nil {
		return 0, err
	}
	return peerBalance.balance, nil
}

// get the balance storage key for the given peer
func balanceKey(peer swarm.Address) string {
	return fmt.Sprintf("balance_%s", peer.String())
}

// getPeerBalance gets the PeerBalance for a given peer
// If not in memory it will try to load it from the state store
// if not found it will initialise it with 0 balance
func (a *Accounting) getPeerBalance(peer swarm.Address) (*PeerBalance, error) {
	a.balancesMu.Lock()
	defer a.balancesMu.Unlock()

	peerBalance, ok := a.balances[peer.String()]
	if !ok {
		// balance not yet in memory, load from state store
		var balance int64
		err := a.store.Get(balanceKey(peer), &balance)
		if err == nil {
			peerBalance = &PeerBalance{
				balance:  balance,
				reserved: 0,
			}
		} else if err == storage.ErrNotFound {
			// no prior records in state store
			peerBalance = &PeerBalance{
				balance:  0,
				reserved: 0,
			}
		} else {
			// other error in state store
			return nil, err
		}

		a.balances[peer.String()] = peerBalance
	}

	return peerBalance, nil
}

func (pb *PeerBalance) expectedBalance() int64 {
	return pb.balance - int64(pb.reserved)
}

func (pb *PeerBalance) expectedDebt() uint64 {
	expectedBalance := pb.expectedBalance()
	if expectedBalance >= 0 {
		return 0
	}
	return uint64(-expectedBalance)
}

// NotifyPayment is called by Settlement when we received payment
func (a *Accounting) NotifyPayment(peer swarm.Address, amount uint64) error {
	balance, err := a.getPeerBalance(peer)
	if err != nil {
		return err
	}

	balance.lock.Lock()
	defer balance.lock.Unlock()

	nextBalance := balance.balance - int64(amount)

	a.logger.Tracef("crediting peer %v with amount %d due to payment, new balance is %d", peer, amount, nextBalance)

	err = a.store.Put(balanceKey(peer), nextBalance)
	if err != nil {
		return fmt.Errorf("failed to persist balance for peer %v: %w", peer, err)
	}

	balance.balance = nextBalance

	return nil
}
