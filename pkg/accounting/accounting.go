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
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	_                     Interface = (*Accounting)(nil)
	balancesPrefix        string    = "accounting_balance_"
	balancesSurplusPrefix string    = "accounting_surplusbalance_"
)

// Interface is the Accounting interface.
type Interface interface {
	// Reserve reserves a portion of the balance for peer and attempts settlements if necessary.
	// Returns an error if the operation risks exceeding the disconnect threshold or an attempted settlement failed.
	//
	// This has to be called (always in combination with Release) before a
	// Credit action to prevent overspending in case of concurrent requests.
	Reserve(ctx context.Context, peer swarm.Address, price uint64) error
	// Release releases the reserved funds.
	Release(peer swarm.Address, price uint64)
	// Credit increases the balance the peer has with us (we "pay" the peer).
	Credit(peer swarm.Address, price uint64) error
	// PrepareDebit returns an accounting Action for the later debit to be executed on and to implement shadowing reserve on the other side
	PrepareDebit(peer swarm.Address, price uint64) Action
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

type Action interface {
	Cleanup()
	Apply() error
}

type debitAction struct {
	accounting     *Accounting
	price          *big.Int
	peer           swarm.Address
	accountingPeer *accountingPeer
	applied        bool
}

type PayFunc func(context.Context, swarm.Address, *big.Int)
type RefreshFunc func(context.Context, swarm.Address, *big.Int, *big.Int)

// accountingPeer holds all in-memory accounting information for one peer.
type accountingPeer struct {
	lock                  sync.Mutex // lock to be held during any accounting action for this peer
	reservedBalance       *big.Int   // amount currently reserved for active peer interaction
	shadowReservedBalance *big.Int   // amount currently reserved for active peer interaction
	paymentThreshold      *big.Int   // the threshold at which the peer expects us to pay
	refreshOngoing        bool
	refreshOngoingLock    sync.Mutex // lets ongoing payment delay new incoming request attempts
	refreshTimestamp      int64
	paymentOngoing        bool // indicate if we are currently settling with the peer
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
	earlyPayment     *big.Int
	payFunction      PayFunc
	refreshFunction  RefreshFunc
	refreshRate      *big.Int
	pricing          pricing.Interface
	metrics          metrics
}

var (
	// ErrOverdraft denotes the expected debt in Reserve would exceed the payment thresholds.
	ErrOverdraft = errors.New("attempted overdraft")
	// ErrDisconnectThresholdExceeded denotes a peer has exceeded the disconnect threshold.
	ErrDisconnectThresholdExceeded = errors.New("disconnect threshold exceeded")
	// ErrPeerNoBalance is the error returned if no balance in store exists for a peer
	ErrPeerNoBalance = errors.New("no balance for peer")
	// ErrOverflow denotes an arithmetic operation overflowed.
	ErrOverflow = errors.New("overflow error")
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
) (*Accounting, error) {
	return &Accounting{
		accountingPeers:  make(map[string]*accountingPeer),
		paymentThreshold: new(big.Int).Set(PaymentThreshold),
		paymentTolerance: new(big.Int).Set(PaymentTolerance),
		earlyPayment:     new(big.Int).Set(EarlyPayment),
		logger:           Logger,
		store:            Store,
		pricing:          Pricing,
		metrics:          newMetrics(),
		refreshRate:      refreshRate,
	}, nil
}

// Reserve reserves a portion of the balance for peer and attempts settlements if necessary.
func (a *Accounting) Reserve(ctx context.Context, peer swarm.Address, price uint64) error {
	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	currentBalance, err := a.Balance(peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			return fmt.Errorf("failed to load balance: %w", err)
		}
	}
	currentDebt := new(big.Int).Neg(currentBalance)
	if currentDebt.Cmp(big.NewInt(0)) < 0 {
		currentDebt.SetInt64(0)
	}

	bigPrice := new(big.Int).SetUint64(price)
	nextReserved := new(big.Int).Add(accountingPeer.reservedBalance, bigPrice)

	expectedDebt := new(big.Int).Add(currentDebt, nextReserved)

	threshold := new(big.Int).Set(accountingPeer.paymentThreshold)
	if threshold.Cmp(a.earlyPayment) > 0 {
		threshold.Sub(threshold, a.earlyPayment)
	} else {
		threshold.SetInt64(0)
	}

	additionalDebt, err := a.SurplusBalance(peer)
	if err != nil {
		return fmt.Errorf("failed to load surplus balance: %w", err)
	}

	// uint64 conversion of surplusbalance is safe because surplusbalance is always positive
	if additionalDebt.Cmp(big.NewInt(0)) < 0 {
		return ErrInvalidValue
	}

	increasedExpectedDebt := new(big.Int).Add(expectedDebt, additionalDebt)

	// If our expected debt is less than earlyPayment away from our payment threshold
	// and we are actually in debt, trigger settlement.
	// we pay early to avoid needlessly blocking request later when concurrent requests occur and we are already close to the payment threshold.
	if increasedExpectedDebt.Cmp(threshold) >= 0 && currentBalance.Cmp(big.NewInt(0)) < 0 {
		err = a.settle(peer, accountingPeer)
		if err != nil {
			return fmt.Errorf("failed to settle with peer %v: %v", peer, err)
		}
	}

	// if expectedDebt would still exceed the paymentThreshold at this point block this request
	// this can happen if there is a large number of concurrent requests to the same peer
	if increasedExpectedDebt.Cmp(accountingPeer.paymentThreshold) > 0 {
		a.metrics.AccountingBlocksCount.Inc()
		return ErrOverdraft
	}

	accountingPeer.reservedBalance = nextReserved
	return nil
}

// Release releases reserved funds.
func (a *Accounting) Release(peer swarm.Address, price uint64) {
	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	bigPrice := new(big.Int).SetUint64(price)

	// NOTE: this should never happen if Reserve and Release calls are paired.
	if bigPrice.Cmp(accountingPeer.reservedBalance) > 0 {
		a.logger.Error("attempting to release more balance than was reserved for peer")
		accountingPeer.reservedBalance.SetUint64(0)
	} else {
		accountingPeer.reservedBalance.Sub(accountingPeer.reservedBalance, bigPrice)
	}
}

// Credit increases the amount of credit we have with the given peer
// (and decreases existing debt).
func (a *Accounting) Credit(peer swarm.Address, price uint64) error {
	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	currentBalance, err := a.Balance(peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			return fmt.Errorf("failed to load balance: %w", err)
		}
	}

	// Calculate next balance by safely decreasing current balance with the price we credit
	nextBalance := new(big.Int).Sub(currentBalance, new(big.Int).SetUint64(price))

	a.logger.Tracef("crediting peer %v with price %d, new balance is %d", peer, price, nextBalance)

	err = a.store.Put(peerBalanceKey(peer), nextBalance)
	if err != nil {
		return fmt.Errorf("failed to persist balance: %w", err)
	}

	a.metrics.TotalCreditedAmount.Add(float64(price))
	a.metrics.CreditEventsCount.Inc()
	return nil
}

// Settle all debt with a peer. The lock on the accountingPeer must be held when
// called.
func (a *Accounting) settle(peer swarm.Address, balance *accountingPeer) error {

	// if no payment ongoing, then timesettle
	// if there is already a timesettle ongoing
	if balance.refreshOngoing && balance.paymentOngoing {
		return nil
	}

	// if either type of payment not ongoing, then
	// if there is already a timesettle ongoing

	// now := time.Now().Unix()
	// timeElapsed := big.NewInt(now - balance.refreshTimestamp)

	oldBalance, err := a.Balance(peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			return fmt.Errorf("failed to load balance: %w", err)
		}
	}

	surplusBalance, err := a.SurplusBalance(peer)
	if err != nil {
		return fmt.Errorf("failed to load balance: %w", err)
	}

	if surplusBalance.Cmp(big.NewInt(0)) < 0 {
		return ErrInvalidValue
	}

	compensatedBalance := new(big.Int).Sub(oldBalance, surplusBalance)

	// Don't do anything if there is no actual debt.
	// This might be the case if the peer owes us and the total reserve for a
	// peer exceeds the payment treshold.

	timeBasedPaymentAmount := new(big.Int).Neg(compensatedBalance)

	if !balance.paymentOngoing && !balance.refreshOngoing && timeBasedPaymentAmount.Cmp(big.NewInt(0)) > 0 {
		balance.refreshOngoing = true
		a.logger.Infof("### %d", timeBasedPaymentAmount)
		balance.refreshOngoingLock.Lock()
		a.logger.Infof("### postlock %d", timeBasedPaymentAmount)

		shadowBalance, err := a.shadowBalance(peer)
		if err != nil {
			balance.refreshOngoingLock.Unlock()
			balance.refreshOngoing = false
			return err
		}

		go a.refreshFunction(context.Background(), peer, timeBasedPaymentAmount, shadowBalance)
	}

	//	paymentAmount := new(big.Int).Neg(oldBalance)

	// if !balance.paymentOngoing && !balance.refreshOngoing && paymentAmount.Cmp(big.NewInt(0)) > 0 {
	// 		balance.paymentOngoing = true
	// 		balance.shadowReservedBalance.Add(balance.shadowReservedBalance, paymentAmount)
	// 		a.logger.Error("sending real payment")
	// 		go a.payFunction(context.Background(), peer, paymentAmount)
	// }

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

// SurplusBalance returns the current balance for the given peer.
func (a *Accounting) SurplusBalance(peer swarm.Address) (balance *big.Int, err error) {
	err = a.store.Get(peerSurplusBalanceKey(peer), &balance)

	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return big.NewInt(0), nil
		}
		return nil, err
	}

	return balance, nil
}

// CompensatedBalance returns balance decreased by surplus balance
func (a *Accounting) CompensatedBalance(peer swarm.Address) (compensated *big.Int, err error) {

	surplus, err := a.SurplusBalance(peer)
	if err != nil {
		return nil, err
	}

	if surplus.Cmp(big.NewInt(0)) < 0 {
		return nil, ErrInvalidValue
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
			// initially assume the peer has the same threshold as us
			paymentThreshold: new(big.Int).Set(a.paymentThreshold),
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
			return false, fmt.Errorf("parse address from key: %s: %v", string(key), err)
		}

		if _, ok := s[addr.String()]; !ok {
			var storevalue *big.Int
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

// Balances gets balances for all peers from store.
func (a *Accounting) CompensatedBalances() (map[string]*big.Int, error) {
	s := make(map[string]*big.Int)

	err := a.store.Iterate(balancesPrefix, func(key, val []byte) (stop bool, err error) {
		addr, err := balanceKeyPeer(key)
		if err != nil {
			return false, fmt.Errorf("parse address from key: %s: %v", string(key), err)
		}
		if _, ok := s[addr.String()]; !ok {
			value, err := a.CompensatedBalance(addr)
			if err != nil {
				return false, fmt.Errorf("get peer %s balance: %v", addr.String(), err)
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
			return false, fmt.Errorf("parse address from key: %s: %v", string(key), err)
		}
		if _, ok := s[addr.String()]; !ok {
			value, err := a.CompensatedBalance(addr)
			if err != nil {
				return false, fmt.Errorf("get peer %s balance: %v", addr.String(), err)
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

// NotifyPaymentThreshold should be called to notify accounting of changes in the payment threshold
func (a *Accounting) NotifyPaymentThreshold(peer swarm.Address, paymentThreshold *big.Int) error {
	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	accountingPeer.paymentThreshold.Set(paymentThreshold)
	return nil
}

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

// ShadowBalance returns the current debt reduced by any potentially debitable amount stored in shadowReservedBalance
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

	if negativeBalance.Cmp(accountingPeer.shadowReservedBalance) < 0 {
		return zero, nil
	}

	shadowBalance = new(big.Int).Sub(negativeBalance, accountingPeer.shadowReservedBalance)

	return shadowBalance, nil
}

func (a *Accounting) NotifyPaymentSent(peer swarm.Address, amount *big.Int, receivedError error) {
	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	accountingPeer.paymentOngoing = false
	accountingPeer.shadowReservedBalance.Sub(accountingPeer.shadowReservedBalance, amount)

	if receivedError != nil {
		a.logger.Warningf("accouting: payment failure %v", receivedError)
		return
	}

	currentBalance, err := a.Balance(peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			a.logger.Warningf("accounting: notifypaymentsent failed to load balance: %v", err)
			return
		}
	}

	// Get nextBalance by safely increasing current balance with price
	nextBalance := new(big.Int).Add(currentBalance, amount)

	a.logger.Tracef("registering payment sent to peer %v with amount %d, new balance is %d", peer, amount, nextBalance)

	err = a.store.Put(peerBalanceKey(peer), nextBalance)
	if err != nil {
		a.logger.Warningf("accounting: notifypaymentsent failed to persist balance: %v", err)
		return
	}
}

func (a *Accounting) NotifyRefreshmentSent(peer swarm.Address, amount *big.Int, timestamp int64, receivedError error) {
	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.refreshOngoingLock.Unlock()

	a.logger.Info("refreshment sent 1")

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	a.logger.Info("refreshment sent 2")

	accountingPeer.refreshOngoing = false

	a.logger.Info("refreshment sent 3")

	if receivedError != nil {
		a.logger.Warningf("accouting: refresh failure %v", receivedError)
		return
	}

	accountingPeer.refreshTimestamp = timestamp

	currentBalance, err := a.Balance(peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			a.logger.Warningf("accounting: notifyrefreshmentsent: failed to load balance: %v", err)
			return
		}
	}

	// Get nextBalance by safely increasing current balance with price
	nextBalance := new(big.Int).Add(currentBalance, amount)

	a.logger.Tracef("registering refreshment sent to peer %v with amount %d, new balance is %d", peer, amount, nextBalance)

	err = a.store.Put(peerBalanceKey(peer), nextBalance)
	if err != nil {
		a.logger.Warningf("accounting: notifyrefreshmentsent failed to persist balance: %v", err)
		return
	}

	a.logger.Info("######### refreshment sent %d", amount)
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

	a.logger.Info("############ refreshment received %d", amount)

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

// NotifyPayment is called by Settlement when we receive a payment.
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

	// if current balance is positive, let's make a partial credit to
	nextBalance := new(big.Int).Sub(currentBalance, amount)

	// Don't allow a payment to put us into debt
	// This is to prevent another node tricking us into settling by settling
	// first (e.g. send a bouncing cheque to trigger an honest cheque in swap).

	a.logger.Tracef("crediting peer %v with amount %d due to payment, new balance is %d", peer, amount, nextBalance)

	err = a.store.Put(peerBalanceKey(peer), nextBalance)
	if err != nil {
		return fmt.Errorf("failed to persist balance: %w", err)
	}

	return nil
}

func (a *Accounting) PrepareDebit(peer swarm.Address, price uint64) Action {
	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.refreshOngoingLock.Lock()
	defer accountingPeer.refreshOngoingLock.Unlock()

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	bigPrice := new(big.Int).SetUint64(price)

	accountingPeer.shadowReservedBalance = new(big.Int).Add(accountingPeer.shadowReservedBalance, bigPrice)

	return &debitAction{
		accounting:     a,
		price:          bigPrice,
		peer:           peer,
		accountingPeer: accountingPeer,
		applied:        false,
	}

}

func (d *debitAction) Apply() error {
	d.accountingPeer.lock.Lock()
	defer d.accountingPeer.lock.Unlock()

	a := d.accounting

	cost := new(big.Int).Set(d.price)
	// see if peer has surplus balance to deduct this transaction of

	surplusBalance, err := a.SurplusBalance(d.peer)
	if err != nil {
		return fmt.Errorf("failed to get surplus balance: %w", err)
	}
	if surplusBalance.Cmp(big.NewInt(0)) > 0 {

		// get new surplus balance after deduct
		newSurplusBalance := new(big.Int).Sub(surplusBalance, cost)

		// if nothing left for debiting, store new surplus balance and return from debit
		if newSurplusBalance.Cmp(big.NewInt(0)) >= 0 {
			a.logger.Tracef("surplus debiting peer %v with value %d, new surplus balance is %d", d.peer, d.price, newSurplusBalance)

			err = a.store.Put(peerSurplusBalanceKey(d.peer), newSurplusBalance)
			if err != nil {
				return fmt.Errorf("failed to persist surplus balance: %w", err)
			}
			// count debit operations, terminate early
			tot, _ := big.NewFloat(0).SetInt(d.price).Float64()

			d.applied = true
			d.accountingPeer.shadowReservedBalance = new(big.Int).Sub(d.accountingPeer.shadowReservedBalance, d.price)
			a.metrics.TotalDebitedAmount.Add(tot)
			a.metrics.DebitEventsCount.Inc()
			return nil
		}

		// if surplus balance didn't cover full transaction, let's continue with leftover part as cost
		debitIncrease := new(big.Int).Sub(d.price, surplusBalance)

		// conversion to uint64 is safe because we know the relationship between the values by now, but let's make a sanity check
		if debitIncrease.Cmp(big.NewInt(0)) <= 0 {
			return fmt.Errorf("sanity check failed for partial debit after surplus balance drawn")
		}
		cost.Set(debitIncrease)

		// if we still have something to debit, than have run out of surplus balance,
		// let's store 0 as surplus balance
		a.logger.Tracef("surplus debiting peer %v with value %d, new surplus balance is 0", d.peer, debitIncrease)

		err = a.store.Put(peerSurplusBalanceKey(d.peer), big.NewInt(0))
		if err != nil {
			return fmt.Errorf("failed to persist surplus balance: %w", err)
		}

	}

	currentBalance, err := a.Balance(d.peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			return fmt.Errorf("failed to load balance: %w", err)
		}
	}

	// Get nextBalance by safely increasing current balance with price
	nextBalance := new(big.Int).Add(currentBalance, cost)

	a.logger.Tracef("debiting peer %v with price %d, new balance is %d", d.peer, d.price, nextBalance)

	err = a.store.Put(peerBalanceKey(d.peer), nextBalance)
	if err != nil {
		return fmt.Errorf("failed to persist balance: %w", err)
	}

	d.applied = true
	d.accountingPeer.shadowReservedBalance = new(big.Int).Sub(d.accountingPeer.shadowReservedBalance, d.price)

	tot, _ := big.NewFloat(0).SetInt(d.price).Float64()

	a.metrics.TotalDebitedAmount.Add(tot)
	a.metrics.DebitEventsCount.Inc()

	if nextBalance.Cmp(new(big.Int).Add(a.paymentThreshold, a.paymentTolerance)) >= 0 {
		// peer too much in debt
		a.metrics.AccountingDisconnectsCount.Inc()
		return p2p.NewBlockPeerError(10000*time.Hour, ErrDisconnectThresholdExceeded)
	}

	return nil
}

func (d *debitAction) Cleanup() {
	if !d.applied {
		d.accountingPeer.lock.Lock()
		defer d.accountingPeer.lock.Unlock()
		d.accountingPeer.shadowReservedBalance = new(big.Int).Sub(d.accountingPeer.shadowReservedBalance, d.price)
	}
}

func (a *Accounting) SetRefreshFunc(f RefreshFunc) {
	a.refreshFunction = f
}

func (a *Accounting) SetPayFunc(f PayFunc) {
	a.payFunction = f
}
