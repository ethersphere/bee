// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package accounting provides functionalities needed
// to do per-peer accounting.
package accounting

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/pricing"
	"github.com/ethersphere/bee/pkg/settlement/pseudosettle"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	_                        Interface = (*Accounting)(nil)
	balancesPrefix                     = "accounting_balance_"
	balancesSurplusPrefix              = "accounting_surplusbalance_"
	balancesOriginatedPrefix           = "accounting_originatedbalance_"
	// fraction of the refresh rate that is the minimum for monetary settlement
	// this value is chosen so that tiny payments are prevented while still allowing small payments in environments with lower payment thresholds
	minimumPaymentDivisor    = int64(5)
	failedSettlementInterval = int64(10) // seconds
)

// Interface is the Accounting interface.
type Interface interface {
	// Credit action to prevent overspending in case of concurrent requests.
	PrepareCredit(peer swarm.Address, price uint64, originated bool) (Action, error)
	// PrepareDebit returns an accounting Action for the later debit to be executed on and to implement shadowing a possibly credited part of reserve on the other side.
	PrepareDebit(peer swarm.Address, price uint64) (Action, error)
	// Balance returns the current balance for the given peer.
	Balance(peer swarm.Address) (*big.Int, error)
	// SurplusBalance returns the current surplus balance for the given peer.
	SurplusBalance(peer swarm.Address) (*big.Int, error)
	// Balances returns balances for all known peers.
	Balances() (map[string]*big.Int, error)
	// CompensatedBalance returns the current balance deducted by current surplus balance for the given peer.
	CompensatedBalance(peer swarm.Address) (*big.Int, error)
	// CompensatedBalances returns the compensated balances for all known peers.
	CompensatedBalances() (map[string]*big.Int, error)
}

// Action represents an accounting action that can be applied
type Action interface {
	// Cleanup cleans up an action. Must be called whether it was applied or not.
	Cleanup()
	// Apply applies an action
	Apply() error
}

// debitAction represents a future debit
type debitAction struct {
	accounting     *Accounting
	price          *big.Int
	peer           swarm.Address
	accountingPeer *accountingPeer
	applied        bool
}

// creditAction represents a future debit
type creditAction struct {
	accounting     *Accounting
	price          *big.Int
	peer           swarm.Address
	accountingPeer *accountingPeer
	originated     bool
	applied        bool
}

// PayFunc is the function used for async monetary settlement
type PayFunc func(context.Context, swarm.Address, *big.Int)

// RefreshFunc is the function used for sync time-based settlement
type RefreshFunc func(context.Context, swarm.Address, *big.Int, *big.Int) (*big.Int, int64, error)

// accountingPeer holds all in-memory accounting information for one peer.
type accountingPeer struct {
	lock                           sync.Mutex // lock to be held during any accounting action for this peer
	reservedBalance                *big.Int   // amount currently reserved for active peer interaction
	shadowReservedBalance          *big.Int   // amount potentially to be debited for active peer interaction
	ghostBalance                   *big.Int   // amount potentially could have been debited for but was not
	paymentThreshold               *big.Int   // the threshold at which the peer expects us to pay
	refreshTimestamp               int64      // last time we attempted time-based settlement
	paymentOngoing                 bool       // indicate if we are currently settling with the peer
	lastSettlementFailureTimestamp int64      // time of last unsuccessful attempt to issue a cheque
	connected                      bool
}

// Accounting is the main implementation of the accounting interface.
type Accounting struct {
	// Mutex for accessing the accountingPeers map.
	accountingPeersMu sync.Mutex
	accountingPeers   map[string]*accountingPeer
	logger            logging.Logger
	store             storage.StateStorer
	// The payment threshold in BZZ we communicate to our peers.
	paymentThreshold *big.Int
	// The amount in BZZ we let peers exceed the payment threshold before we
	// disconnect them.
	paymentTolerance *big.Int
	// Start settling when reserve plus debt reaches this close to threshold.
	earlyPayment *big.Int
	// Limit to disconnect peer after going in debt over
	disconnectLimit *big.Int
	// function used for monetary settlement
	payFunction PayFunc
	// function used for time settlement
	refreshFunction RefreshFunc
	// allowance based on time used in pseudo settle
	refreshRate *big.Int
	// lower bound for the value of issued cheques
	minimumPayment *big.Int
	pricing        pricing.Interface
	metrics        metrics
	wg             sync.WaitGroup
	p2p            p2p.Service
	timeNow        func() time.Time
}

var (
	// ErrOverdraft denotes the expected debt in Reserve would exceed the payment thresholds.
	ErrOverdraft = errors.New("attempted overdraft")
	// ErrDisconnectThresholdExceeded denotes a peer has exceeded the disconnect threshold.
	ErrDisconnectThresholdExceeded = errors.New("disconnect threshold exceeded")
	// ErrPeerNoBalance is the error returned if no balance in store exists for a peer
	ErrPeerNoBalance = errors.New("no balance for peer")
	// ErrInvalidValue denotes an invalid value read from store
	ErrInvalidValue = errors.New("invalid value")
)

// NewAccounting creates a new Accounting instance with the provided options.
func NewAccounting(
	PaymentThreshold,
	PaymentTolerance,
	EarlyPayment *big.Int,
	Logger logging.Logger,
	Store storage.StateStorer,
	Pricing pricing.Interface,
	refreshRate *big.Int,
	p2pService p2p.Service,

) (*Accounting, error) {
	return &Accounting{
		accountingPeers:  make(map[string]*accountingPeer),
		paymentThreshold: new(big.Int).Set(PaymentThreshold),
		paymentTolerance: new(big.Int).Set(PaymentTolerance),
		earlyPayment:     new(big.Int).Set(EarlyPayment),
		disconnectLimit:  new(big.Int).Add(PaymentThreshold, PaymentTolerance),
		logger:           Logger,
		store:            Store,
		pricing:          Pricing,
		metrics:          newMetrics(),
		refreshRate:      refreshRate,
		timeNow:          time.Now,
		minimumPayment:   new(big.Int).Div(refreshRate, big.NewInt(minimumPaymentDivisor)),
		p2p:              p2pService,
	}, nil
}

func (a *Accounting) getIncreasedExpectedDebt(peer swarm.Address, accountingPeer *accountingPeer, bigPrice *big.Int) (*big.Int, *big.Int, error) {
	nextReserved := new(big.Int).Add(accountingPeer.reservedBalance, bigPrice)

	currentBalance, err := a.Balance(peer)
	if err != nil && !errors.Is(err, ErrPeerNoBalance) {
		return nil, nil, fmt.Errorf("failed to load balance: %w", err)
	}
	currentDebt := new(big.Int).Neg(currentBalance)
	if currentDebt.Cmp(big.NewInt(0)) < 0 {
		currentDebt.SetInt64(0)
	}

	// debt if all reserved operations are successfully credited excluding debt created by surplus balance
	expectedDebt := new(big.Int).Add(currentDebt, nextReserved)

	// additionalDebt is debt created by incoming payments which we don't consider debt for monetary settlement purposes
	additionalDebt, err := a.SurplusBalance(peer)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load surplus balance: %w", err)
	}

	// debt if all reserved operations are successfully credited including debt created by surplus balance
	return new(big.Int).Add(expectedDebt, additionalDebt), currentBalance, nil
}

func (a *Accounting) PrepareCredit(peer swarm.Address, price uint64, originated bool) (Action, error) {
	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	if !accountingPeer.connected {
		return nil, fmt.Errorf("connection not initialized yet")
	}

	a.metrics.AccountingReserveCount.Inc()
	bigPrice := new(big.Int).SetUint64(price)

	threshold := new(big.Int).Set(accountingPeer.paymentThreshold)
	if threshold.Cmp(a.earlyPayment) > 0 {
		threshold.Sub(threshold, a.earlyPayment)
	} else {
		threshold.SetInt64(0)
	}

	// debt if all reserved operations are successfully credited including debt created by surplus balance
	increasedExpectedDebt, currentBalance, err := a.getIncreasedExpectedDebt(peer, accountingPeer, bigPrice)
	if err != nil {
		return nil, err
	}
	// debt if all reserved operations are successfully credited and all shadow reserved operations are debited including debt created by surplus balance
	// in other words this the debt the other node sees if everything pending is successful
	increasedExpectedDebtReduced := new(big.Int).Sub(increasedExpectedDebt, accountingPeer.shadowReservedBalance)

	// If our expected debt reduced by what could have been credited on the other side already is less than earlyPayment away from our payment threshold
	// and we are actually in debt, trigger settlement.
	// we pay early to avoid needlessly blocking request later when concurrent requests occur and we are already close to the payment threshold.

	if increasedExpectedDebtReduced.Cmp(threshold) >= 0 && currentBalance.Cmp(big.NewInt(0)) < 0 {
		err = a.settle(peer, accountingPeer)
		if err != nil {
			return nil, fmt.Errorf("failed to settle with peer %v: %w", peer, err)
		}

		increasedExpectedDebt, _, err = a.getIncreasedExpectedDebt(peer, accountingPeer, bigPrice)
		if err != nil {
			return nil, err
		}
	}

	// if expectedDebt would still exceed the paymentThreshold at this point block this request
	// this can happen if there is a large number of concurrent requests to the same peer
	if increasedExpectedDebt.Cmp(accountingPeer.paymentThreshold) > 0 {
		a.metrics.AccountingBlocksCount.Inc()
		return nil, ErrOverdraft
	}

	accountingPeer.reservedBalance = new(big.Int).Add(accountingPeer.reservedBalance, bigPrice)
	return &creditAction{
		accounting:     a,
		price:          bigPrice,
		peer:           peer,
		accountingPeer: accountingPeer,
		originated:     originated,
	}, nil
}

func (c *creditAction) Apply() error {
	c.accountingPeer.lock.Lock()
	defer c.accountingPeer.lock.Unlock()

	currentBalance, err := c.accounting.Balance(c.peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			return fmt.Errorf("failed to load balance: %w", err)
		}
	}

	// Calculate next balance by decreasing current balance with the price we credit
	nextBalance := new(big.Int).Sub(currentBalance, c.price)

	c.accounting.logger.Tracef("crediting peer %v with price %d, new balance is %d", c.peer, c.price, nextBalance)

	err = c.accounting.store.Put(peerBalanceKey(c.peer), nextBalance)
	if err != nil {
		return fmt.Errorf("failed to persist balance: %w", err)
	}

	c.accounting.metrics.TotalCreditedAmount.Add(float64(c.price.Int64()))
	c.accounting.metrics.CreditEventsCount.Inc()

	if !c.originated {
		return nil
	}

	originBalance, err := c.accounting.OriginatedBalance(c.peer)
	if err != nil && !errors.Is(err, ErrPeerNoBalance) {
		return fmt.Errorf("failed to load originated balance: %w", err)
	}

	// Calculate next balance by decreasing current balance with the price we credit
	nextOriginBalance := new(big.Int).Sub(originBalance, c.price)

	c.accounting.logger.Tracef("crediting peer %v with price %d, new originated balance is %d", c.peer, c.price, nextOriginBalance)

	zero := big.NewInt(0)
	// only consider negative balance for limiting originated balance
	if nextBalance.Cmp(zero) > 0 {
		nextBalance.Set(zero)
	}

	// If originated balance is more into the negative domain, set it to balance
	if nextOriginBalance.Cmp(nextBalance) < 0 {
		nextOriginBalance.Set(nextBalance)
		c.accounting.logger.Tracef("decreasing originated balance to peer %v to current balance %d", c.peer, nextOriginBalance)
	}

	err = c.accounting.store.Put(originatedBalanceKey(c.peer), nextOriginBalance)
	if err != nil {
		return fmt.Errorf("failed to persist originated balance: %w", err)
	}

	c.accounting.metrics.TotalOriginatedCreditedAmount.Add(float64(c.price.Int64()))
	c.accounting.metrics.OriginatedCreditEventsCount.Inc()

	if c.price.Cmp(c.accountingPeer.reservedBalance) > 0 {
		c.accounting.logger.Error("attempting to release more balance than was reserved for peer")
		c.accountingPeer.reservedBalance.SetUint64(0)
	} else {
		c.accountingPeer.reservedBalance.Sub(c.accountingPeer.reservedBalance, c.price)
	}

	c.applied = true
	return nil
}

func (c *creditAction) Cleanup() {
	if c.applied {
		return
	}

	c.accountingPeer.lock.Lock()
	defer c.accountingPeer.lock.Unlock()

	if c.price.Cmp(c.accountingPeer.reservedBalance) > 0 {
		c.accounting.logger.Error("attempting to release more balance than was reserved for peer")
		c.accountingPeer.reservedBalance.SetUint64(0)
	} else {
		c.accountingPeer.reservedBalance.Sub(c.accountingPeer.reservedBalance, c.price)
	}
}

// Settle all debt with a peer. The lock on the accountingPeer must be held when
// called.
func (a *Accounting) settle(peer swarm.Address, balance *accountingPeer) error {
	now := a.timeNow().Unix()
	timeElapsed := now - balance.refreshTimestamp

	oldBalance, err := a.Balance(peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			return fmt.Errorf("failed to load balance: %w", err)
		}
	}

	// compute the debt including debt created by incoming payments
	compensatedBalance, err := a.CompensatedBalance(peer)
	if err != nil {
		return err
	}

	paymentAmount := new(big.Int).Neg(compensatedBalance)
	// Don't do anything if there is no actual debt or no time passed since last refreshment attempt
	// This might be the case if the peer owes us and the total reserve for a peer exceeds the payment threshold.
	if paymentAmount.Cmp(big.NewInt(0)) > 0 && timeElapsed > 0 {
		shadowBalance, err := a.shadowBalance(peer)
		if err != nil {
			return err
		}

		acceptedAmount, timestamp, err := a.refreshFunction(context.Background(), peer, paymentAmount, shadowBalance)
		if err != nil {
			if !errors.Is(err, pseudosettle.ErrSettlementTooSoon) && !errors.Is(err, p2p.ErrPeerNotFound) {
				fmt.Println("\\\\\\|||///\\\\\\|||///\\\\\\|||///")
				fmt.Println("\\\\\\|||///\\\\\\|||///")
				fmt.Println(err)
				fmt.Println(errors.Is(err, pseudosettle.ErrSettlementTooSoon))
				fmt.Println("\\\\\\|||///\\\\\\|||///")
				fmt.Println("\\\\\\|||///\\\\\\|||///\\\\\\|||///")
				a.metrics.AccountingDisconnectsEnforceRefreshCount.Inc()
				_ = a.blocklist(peer, 1, "failed to refresh")
				return fmt.Errorf("refresh failure: %w", err)
			} else {
				return fmt.Errorf("refresh failure: %w", err)
			}
		}

		balance.refreshTimestamp = timestamp

		oldBalance = new(big.Int).Add(oldBalance, acceptedAmount)

		a.logger.Tracef("registering refreshment sent to peer %v with amount %d, new balance is %d", peer, acceptedAmount, oldBalance)

		err = a.store.Put(peerBalanceKey(peer), oldBalance)
		if err != nil {
			return fmt.Errorf("settle: failed to persist balance: %w", err)
		}

		err = a.decreaseOriginatedBalanceTo(peer, oldBalance)
		if err != nil {
			return fmt.Errorf("settle: failed to decrease originated balance: %w", err)
		}
	}

	if a.payFunction != nil && !balance.paymentOngoing {

		difference := now - balance.lastSettlementFailureTimestamp
		if difference > failedSettlementInterval {

			// if there is no monetary settlement happening, check if there is something to settle
			// compute debt excluding debt created by incoming payments
			originatedBalance, err := a.OriginatedBalance(peer)
			if err != nil {
				if !errors.Is(err, ErrPeerNoBalance) {
					return fmt.Errorf("failed to load originated balance to settle: %w", err)
				}
			}

			paymentAmount := new(big.Int).Neg(originatedBalance)
			// if the remaining debt is still larger than some minimum amount, trigger monetary settlement
			if paymentAmount.Cmp(a.minimumPayment) >= 0 {
				balance.paymentOngoing = true
				// add settled amount to shadow reserve before sending it
				balance.shadowReservedBalance.Add(balance.shadowReservedBalance, paymentAmount)
				a.wg.Add(1)
				go a.payFunction(context.Background(), peer, paymentAmount)
			}
		}
	}

	return nil
}

// Balance returns the current balance for the given peer.
func (a *Accounting) Balance(peer swarm.Address) (balance *big.Int, err error) {
	err = a.store.Get(peerBalanceKey(peer), &balance)

	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return big.NewInt(0), ErrPeerNoBalance
		}
		return nil, err
	}

	return balance, nil
}

// Balance returns the current balance for the given peer.
func (a *Accounting) OriginatedBalance(peer swarm.Address) (balance *big.Int, err error) {
	err = a.store.Get(originatedBalanceKey(peer), &balance)

	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return big.NewInt(0), ErrPeerNoBalance
		}
		return nil, err
	}

	return balance, nil
}

// SurplusBalance returns the current balance for the given peer.
func (a *Accounting) SurplusBalance(peer swarm.Address) (balance *big.Int, err error) {
	err = a.store.Get(peerSurplusBalanceKey(peer), &balance)

	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return big.NewInt(0), nil
		}
		return nil, err
	}

	if balance.Cmp(big.NewInt(0)) < 0 {
		return nil, ErrInvalidValue
	}

	return balance, nil
}

// CompensatedBalance returns balance decreased by surplus balance
func (a *Accounting) CompensatedBalance(peer swarm.Address) (compensated *big.Int, err error) {
	surplus, err := a.SurplusBalance(peer)
	if err != nil {
		return nil, err
	}

	balance, err := a.Balance(peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			return nil, err
		}
	}

	// if surplus is 0 and peer has no balance, propagate ErrPeerNoBalance
	if surplus.Cmp(big.NewInt(0)) == 0 && errors.Is(err, ErrPeerNoBalance) {
		return nil, err
	}
	// Compensated balance is balance decreased by surplus balance
	compensated = new(big.Int).Sub(balance, surplus)

	return compensated, nil
}

// peerBalanceKey returns the balance storage key for the given peer.
func peerBalanceKey(peer swarm.Address) string {
	return fmt.Sprintf("%s%s", balancesPrefix, peer.String())
}

// peerSurplusBalanceKey returns the surplus balance storage key for the given peer
func peerSurplusBalanceKey(peer swarm.Address) string {
	return fmt.Sprintf("%s%s", balancesSurplusPrefix, peer.String())
}

func originatedBalanceKey(peer swarm.Address) string {
	return fmt.Sprintf("%s%s", balancesOriginatedPrefix, peer.String())
}

// getAccountingPeer returns the accountingPeer for a given swarm address.
// If not found in memory it will initialize it.
func (a *Accounting) getAccountingPeer(peer swarm.Address) *accountingPeer {
	a.accountingPeersMu.Lock()
	defer a.accountingPeersMu.Unlock()

	peerData, ok := a.accountingPeers[peer.String()]
	if !ok {
		peerData = &accountingPeer{
			reservedBalance:       big.NewInt(0),
			shadowReservedBalance: big.NewInt(0),
			ghostBalance:          big.NewInt(0),
			// initially assume the peer has the same threshold as us
			paymentThreshold: new(big.Int).Set(a.paymentThreshold),
			connected:        false,
		}
		a.accountingPeers[peer.String()] = peerData
	}

	return peerData
}

// Balances gets balances for all peers from store.
func (a *Accounting) Balances() (map[string]*big.Int, error) {
	s := make(map[string]*big.Int)

	err := a.store.Iterate(balancesPrefix, func(key, val []byte) (stop bool, err error) {
		addr, err := balanceKeyPeer(key)
		if err != nil {
			return false, fmt.Errorf("parse address from key: %s: %w", string(key), err)
		}

		if _, ok := s[addr.String()]; !ok {
			var storevalue *big.Int
			err = a.store.Get(peerBalanceKey(addr), &storevalue)
			if err != nil {
				return false, fmt.Errorf("get peer %s balance: %w", addr.String(), err)
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

// Balances gets balances for all peers from store.
func (a *Accounting) CompensatedBalances() (map[string]*big.Int, error) {
	s := make(map[string]*big.Int)

	err := a.store.Iterate(balancesPrefix, func(key, val []byte) (stop bool, err error) {
		addr, err := balanceKeyPeer(key)
		if err != nil {
			return false, fmt.Errorf("parse address from key: %s: %w", string(key), err)
		}
		if _, ok := s[addr.String()]; !ok {
			value, err := a.CompensatedBalance(addr)
			if err != nil {
				return false, fmt.Errorf("get peer %s balance: %w", addr.String(), err)
			}

			s[addr.String()] = value
		}

		return false, nil
	})

	if err != nil {
		return nil, err
	}

	err = a.store.Iterate(balancesSurplusPrefix, func(key, val []byte) (stop bool, err error) {
		addr, err := surplusBalanceKeyPeer(key)
		if err != nil {
			return false, fmt.Errorf("parse address from key: %s: %w", string(key), err)
		}
		if _, ok := s[addr.String()]; !ok {
			value, err := a.CompensatedBalance(addr)
			if err != nil {
				return false, fmt.Errorf("get peer %s balance: %w", addr.String(), err)
			}

			s[addr.String()] = value
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

func surplusBalanceKeyPeer(key []byte) (swarm.Address, error) {
	k := string(key)

	split := strings.SplitAfter(k, balancesSurplusPrefix)
	if len(split) != 2 {
		return swarm.ZeroAddress, errors.New("no peer in key")
	}

	addr, err := swarm.ParseHexAddress(split[1])
	if err != nil {
		return swarm.ZeroAddress, err
	}

	return addr, nil
}

// PeerDebt returns the positive part of the sum of the outstanding balance and the shadow reserve
func (a *Accounting) PeerDebt(peer swarm.Address) (*big.Int, error) {

	accountingPeer := a.getAccountingPeer(peer)
	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	balance := new(big.Int)
	zero := big.NewInt(0)

	err := a.store.Get(peerBalanceKey(peer), &balance)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
		balance = big.NewInt(0)
	}

	peerDebt := new(big.Int).Add(balance, accountingPeer.shadowReservedBalance)

	if peerDebt.Cmp(zero) < 0 {
		return zero, nil
	}

	return peerDebt, nil
}

// peerLatentDebt returns the sum of the positive part of the outstanding balance, shadow reserve and the ghost balance
func (a *Accounting) peerLatentDebt(peer swarm.Address) (*big.Int, error) {

	accountingPeer := a.getAccountingPeer(peer)

	balance := new(big.Int)
	zero := big.NewInt(0)

	err := a.store.Get(peerBalanceKey(peer), &balance)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
		balance = big.NewInt(0)
	}

	if balance.Cmp(zero) < 0 {
		balance.Set(zero)
	}

	peerDebt := new(big.Int).Add(balance, accountingPeer.shadowReservedBalance)
	peerLatentDebt := new(big.Int).Add(peerDebt, accountingPeer.ghostBalance)

	if peerLatentDebt.Cmp(zero) < 0 {
		return zero, nil
	}

	return peerLatentDebt, nil
}

// shadowBalance returns the current debt reduced by any potentially debitable amount stored in shadowReservedBalance
// this represents how much less our debt could potentially be seen by the other party if it's ahead with processing credits corresponding to our shadow reserve
func (a *Accounting) shadowBalance(peer swarm.Address) (shadowBalance *big.Int, err error) {
	accountingPeer := a.getAccountingPeer(peer)
	balance := new(big.Int)
	zero := big.NewInt(0)

	err = a.store.Get(peerBalanceKey(peer), &balance)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return zero, nil
		}
		return nil, err
	}

	if balance.Cmp(zero) >= 0 {
		return zero, nil
	}

	negativeBalance := new(big.Int).Neg(balance)

	surplusBalance, err := a.SurplusBalance(peer)
	if err != nil {
		return nil, err
	}

	debt := new(big.Int).Add(negativeBalance, surplusBalance)

	if debt.Cmp(accountingPeer.shadowReservedBalance) < 0 {
		return zero, nil
	}

	shadowBalance = new(big.Int).Sub(negativeBalance, accountingPeer.shadowReservedBalance)

	return shadowBalance, nil
}

// NotifyPaymentSent is triggered by async monetary settlement to update our balance and remove it's price from the shadow reserve
func (a *Accounting) NotifyPaymentSent(peer swarm.Address, amount *big.Int, receivedError error) {
	defer a.wg.Done()
	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	accountingPeer.paymentOngoing = false
	// decrease shadow reserve by payment value
	accountingPeer.shadowReservedBalance.Sub(accountingPeer.shadowReservedBalance, amount)

	if receivedError != nil {
		accountingPeer.lastSettlementFailureTimestamp = a.timeNow().Unix()
		a.logger.Warningf("accounting: payment failure %v", receivedError)
		return
	}

	currentBalance, err := a.Balance(peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			a.logger.Errorf("accounting: notifypaymentsent failed to load balance: %v", err)
			return
		}
	}

	// Get nextBalance by increasing current balance with price
	nextBalance := new(big.Int).Add(currentBalance, amount)

	a.logger.Tracef("registering payment sent to peer %v with amount %d, new balance is %d", peer, amount, nextBalance)

	err = a.store.Put(peerBalanceKey(peer), nextBalance)
	if err != nil {
		a.logger.Errorf("accounting: notifypaymentsent failed to persist balance: %v", err)
		return
	}

	err = a.decreaseOriginatedBalanceBy(peer, amount)
	if err != nil {
		a.logger.Warningf("accounting: notifypaymentsent failed to decrease originated balance: %v", err)
	}

}

// NotifyPaymentThreshold should be called to notify accounting of changes in the payment threshold
func (a *Accounting) NotifyPaymentThreshold(peer swarm.Address, paymentThreshold *big.Int) error {
	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	accountingPeer.paymentThreshold.Set(paymentThreshold)
	return nil
}

// NotifyPayment is called by Settlement when we receive a payment.
func (a *Accounting) NotifyPaymentReceived(peer swarm.Address, amount *big.Int) error {
	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	currentBalance, err := a.Balance(peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			return err
		}
	}

	// if balance is already negative or zero, we credit full amount received to surplus balance and terminate early
	if currentBalance.Cmp(big.NewInt(0)) <= 0 {
		surplus, err := a.SurplusBalance(peer)
		if err != nil {
			return fmt.Errorf("failed to get surplus balance: %w", err)
		}
		increasedSurplus := new(big.Int).Add(surplus, amount)

		a.logger.Tracef("surplus crediting peer %v with amount %d due to payment, new surplus balance is %d", peer, amount, increasedSurplus)

		err = a.store.Put(peerSurplusBalanceKey(peer), increasedSurplus)
		if err != nil {
			return fmt.Errorf("failed to persist surplus balance: %w", err)
		}

		return nil
	}

	// if current balance is positive, let's make a partial credit to
	newBalance := new(big.Int).Sub(currentBalance, amount)

	// Don't allow a payment to put us into debt
	// This is to prevent another node tricking us into settling by settling
	// first (e.g. send a bouncing cheque to trigger an honest cheque in swap).
	nextBalance := newBalance
	if newBalance.Cmp(big.NewInt(0)) < 0 {
		nextBalance = big.NewInt(0)
	}

	a.logger.Tracef("crediting peer %v with amount %d due to payment, new balance is %d", peer, amount, nextBalance)

	err = a.store.Put(peerBalanceKey(peer), nextBalance)
	if err != nil {
		return fmt.Errorf("failed to persist balance: %w", err)
	}

	// If payment would have put us into debt, rather, let's add to surplusBalance,
	// so as that an oversettlement attempt creates balance for future forwarding services
	// charges to be deducted of
	if newBalance.Cmp(big.NewInt(0)) < 0 {
		surplusGrowth := new(big.Int).Sub(amount, currentBalance)

		surplus, err := a.SurplusBalance(peer)
		if err != nil {
			return fmt.Errorf("failed to get surplus balance: %w", err)
		}
		increasedSurplus := new(big.Int).Add(surplus, surplusGrowth)

		a.logger.Tracef("surplus crediting peer %v with amount %d due to refreshment, new surplus balance is %d", peer, surplusGrowth, increasedSurplus)

		err = a.store.Put(peerSurplusBalanceKey(peer), increasedSurplus)
		if err != nil {
			return fmt.Errorf("failed to persist surplus balance: %w", err)
		}
	}

	return nil
}

// NotifyRefreshmentReceived is called by pseudosettle when we receive a time based settlement.
func (a *Accounting) NotifyRefreshmentReceived(peer swarm.Address, amount *big.Int) error {
	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	currentBalance, err := a.Balance(peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			return err
		}
	}

	// Get nextBalance by increasing current balance with amount
	nextBalance := new(big.Int).Sub(currentBalance, amount)

	// We allow a refreshment to potentially put us into debt as it was previously negotiated and be limited to the peer's outstanding debt plus shadow reserve
	a.logger.Tracef("crediting peer %v with amount %d due to payment, new balance is %d", peer, amount, nextBalance)

	err = a.store.Put(peerBalanceKey(peer), nextBalance)
	if err != nil {
		return fmt.Errorf("failed to persist balance: %w", err)
	}

	return nil
}

// PrepareDebit prepares a debit operation by increasing the shadowReservedBalance
func (a *Accounting) PrepareDebit(peer swarm.Address, price uint64) (Action, error) {
	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	if !accountingPeer.connected {
		return nil, fmt.Errorf("connection not initialized yet")
	}

	bigPrice := new(big.Int).SetUint64(price)

	accountingPeer.shadowReservedBalance = new(big.Int).Add(accountingPeer.shadowReservedBalance, bigPrice)

	return &debitAction{
		accounting:     a,
		price:          bigPrice,
		peer:           peer,
		accountingPeer: accountingPeer,
		applied:        false,
	}, nil
}

func (a *Accounting) increaseBalance(peer swarm.Address, accountingPeer *accountingPeer, price *big.Int) (*big.Int, error) {
	cost := new(big.Int).Set(price)
	// see if peer has surplus balance to deduct this transaction of

	surplusBalance, err := a.SurplusBalance(peer)
	if err != nil {
		return nil, fmt.Errorf("failed to get surplus balance: %w", err)
	}

	if surplusBalance.Cmp(big.NewInt(0)) > 0 {
		// get new surplus balance after deduct
		newSurplusBalance := new(big.Int).Sub(surplusBalance, cost)

		// if nothing left for debiting, store new surplus balance and return from debit
		if newSurplusBalance.Cmp(big.NewInt(0)) >= 0 {
			a.logger.Tracef("surplus debiting peer %v with value %d, new surplus balance is %d", peer, price, newSurplusBalance)

			err = a.store.Put(peerSurplusBalanceKey(peer), newSurplusBalance)
			if err != nil {
				return nil, fmt.Errorf("failed to persist surplus balance: %w", err)
			}

			return a.Balance(peer)
		}

		// if surplus balance didn't cover full transaction, let's continue with leftover part as cost
		debitIncrease := new(big.Int).Sub(price, surplusBalance)

		// a sanity check
		if debitIncrease.Cmp(big.NewInt(0)) <= 0 {
			return nil, fmt.Errorf("sanity check failed for partial debit after surplus balance drawn")
		}
		cost.Set(debitIncrease)

		// if we still have something to debit, than have run out of surplus balance,
		// let's store 0 as surplus balance
		a.logger.Tracef("surplus debiting peer %v with value %d, new surplus balance is 0", peer, debitIncrease)

		err = a.store.Put(peerSurplusBalanceKey(peer), big.NewInt(0))
		if err != nil {
			return nil, fmt.Errorf("failed to persist surplus balance: %w", err)
		}
	}

	currentBalance, err := a.Balance(peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			return nil, fmt.Errorf("failed to load balance: %w", err)
		}
	}

	// Get nextBalance by increasing current balance with price
	nextBalance := new(big.Int).Add(currentBalance, cost)

	a.logger.Tracef("debiting peer %v with price %d, new balance is %d", peer, price, nextBalance)

	err = a.store.Put(peerBalanceKey(peer), nextBalance)
	if err != nil {
		return nil, fmt.Errorf("failed to persist balance: %w", err)
	}

	err = a.decreaseOriginatedBalanceTo(peer, nextBalance)
	if err != nil {
		a.logger.Warningf("increase balance: failed to decrease originated balance: %v", err)
	}

	return nextBalance, nil
}

// Apply applies the debit operation and decreases the shadowReservedBalance
func (d *debitAction) Apply() error {
	d.accountingPeer.lock.Lock()
	defer d.accountingPeer.lock.Unlock()

	a := d.accounting

	cost := new(big.Int).Set(d.price)

	nextBalance, err := d.accounting.increaseBalance(d.peer, d.accountingPeer, cost)
	if err != nil {
		return err
	}

	d.applied = true
	d.accountingPeer.shadowReservedBalance = new(big.Int).Sub(d.accountingPeer.shadowReservedBalance, d.price)

	tot, _ := big.NewFloat(0).SetInt(d.price).Float64()

	a.metrics.TotalDebitedAmount.Add(tot)
	a.metrics.DebitEventsCount.Inc()

	if nextBalance.Cmp(a.disconnectLimit) >= 0 {
		// peer too much in debt
		a.metrics.AccountingDisconnectsOverdrawCount.Inc()

		disconnectFor, err := a.blocklistUntil(d.peer, 1)
		if err != nil {
			return p2p.NewBlockPeerError(1*time.Minute, ErrDisconnectThresholdExceeded)
		}
		return p2p.NewBlockPeerError(time.Duration(disconnectFor)*time.Second, ErrDisconnectThresholdExceeded)

	}

	return nil
}

// Cleanup reduces shadow reserve if and only if debitaction have not been applied
func (d *debitAction) Cleanup() {
	if d.applied {
		return
	}

	d.accountingPeer.lock.Lock()
	defer d.accountingPeer.lock.Unlock()
	a := d.accounting
	d.accountingPeer.shadowReservedBalance = new(big.Int).Sub(d.accountingPeer.shadowReservedBalance, d.price)
	d.accountingPeer.ghostBalance = new(big.Int).Add(d.accountingPeer.ghostBalance, d.price)
	if d.accountingPeer.ghostBalance.Cmp(a.disconnectLimit) > 0 {
		a.metrics.AccountingDisconnectsGhostOverdrawCount.Inc()
		_ = a.blocklist(d.peer, 1, "ghost overdraw")
	}
}

func (a *Accounting) blocklistUntil(peer swarm.Address, multiplier int64) (int64, error) {

	debt, err := a.peerLatentDebt(peer)
	if err != nil {
		return 0, err
	}

	if debt.Cmp(a.refreshRate) < 0 {
		debt.Set(a.refreshRate)
	}

	additionalDebt := new(big.Int).Add(debt, a.paymentThreshold)

	multiplyDebt := new(big.Int).Mul(additionalDebt, big.NewInt(multiplier))

	k := new(big.Int).Div(multiplyDebt, a.refreshRate)

	kInt := k.Int64()

	return kInt, nil
}

func (a *Accounting) blocklist(peer swarm.Address, multiplier int64, reason string) error {

	disconnectFor, err := a.blocklistUntil(peer, multiplier)
	if err != nil {
		return a.p2p.Blocklist(peer, 1*time.Minute, reason)
	}

	return a.p2p.Blocklist(peer, time.Duration(disconnectFor)*time.Second, reason)
}

func (a *Accounting) Connect(peer swarm.Address) {
	accountingPeer := a.getAccountingPeer(peer)
	zero := big.NewInt(0)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	accountingPeer.connected = true
	accountingPeer.shadowReservedBalance.Set(zero)
	accountingPeer.ghostBalance.Set(zero)
	accountingPeer.reservedBalance.Set(zero)

	err := a.store.Put(peerBalanceKey(peer), zero)
	if err != nil {
		a.logger.Errorf("failed to persist balance: %w", err)
	}

	err = a.store.Put(peerSurplusBalanceKey(peer), zero)
	if err != nil {
		a.logger.Errorf("failed to persist surplus balance: %w", err)
	}
}

// decreaseOriginatedBalanceTo decreases the originated balance to provided limit or 0 if limit is positive
func (a *Accounting) decreaseOriginatedBalanceTo(peer swarm.Address, limit *big.Int) error {

	zero := big.NewInt(0)

	toSet := new(big.Int).Set(limit)

	originatedBalance, err := a.OriginatedBalance(peer)
	if err != nil && !errors.Is(err, ErrPeerNoBalance) {
		return fmt.Errorf("failed to load originated balance: %w", err)
	}

	if toSet.Cmp(zero) > 0 {
		toSet.Set(zero)
	}

	// If originated balance is more into the negative domain, set it to limit
	if originatedBalance.Cmp(toSet) < 0 {
		err = a.store.Put(originatedBalanceKey(peer), toSet)
		if err != nil {
			return fmt.Errorf("failed to persist originated balance: %w", err)
		}
		a.logger.Tracef("decreasing originated balance to peer %v to current balance %d", peer, toSet)
	}

	return nil
}

// decreaseOriginatedBalanceTo decreases the originated balance by provided amount even below 0
func (a *Accounting) decreaseOriginatedBalanceBy(peer swarm.Address, amount *big.Int) error {

	originatedBalance, err := a.OriginatedBalance(peer)
	if err != nil && !errors.Is(err, ErrPeerNoBalance) {
		return fmt.Errorf("failed to load balance: %w", err)
	}

	// Move originated balance into the positive domain by amount
	newOriginatedBalance := new(big.Int).Add(originatedBalance, amount)

	err = a.store.Put(originatedBalanceKey(peer), newOriginatedBalance)
	if err != nil {
		return fmt.Errorf("failed to persist originated balance: %w", err)
	}
	a.logger.Tracef("decreasing originated balance to peer %v by amount %d to current balance %d", peer, amount, newOriginatedBalance)

	return nil
}

func (a *Accounting) Disconnect(peer swarm.Address) {
	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	if accountingPeer.connected {
		disconnectFor, err := a.blocklistUntil(peer, 1)
		if err != nil {
			disconnectFor = int64(60)
		}
		accountingPeer.connected = false
		_ = a.p2p.Blocklist(peer, time.Duration(disconnectFor)*time.Second, "disconnected")
	}
}

func (a *Accounting) SetRefreshFunc(f RefreshFunc) {
	a.refreshFunction = f
}

func (a *Accounting) SetPayFunc(f PayFunc) {
	a.payFunction = f
}

// Close hangs up running websockets on shutdown.
func (a *Accounting) Close() error {
	a.wg.Wait()
	return nil
}
