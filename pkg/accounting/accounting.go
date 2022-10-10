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

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/pricing"
	"github.com/ethersphere/bee/pkg/settlement/pseudosettle"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "accounting"

const (
	linearCheckpointNumber = 1800
	linearCheckpointStep   = 100
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
	// PrepareCredit action to prevent overspending in case of concurrent requests.
	PrepareCredit(ctx context.Context, peer swarm.Address, price uint64, originated bool) (Action, error)
	// PrepareDebit returns an accounting Action for the later debit to be executed on and to implement shadowing a possibly credited part of reserve on the other side.
	PrepareDebit(ctx context.Context, peer swarm.Address, price uint64) (Action, error)
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
	// PeerAccounting returns the associated values for all known peers
	PeerAccounting() (map[string]PeerInfo, error)
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
type RefreshFunc func(context.Context, swarm.Address, *big.Int)

// Mutex is a drop in replacement for the sync.Mutex
// it will not lock if the context is expired
type Mutex struct {
	mu chan struct{}
}

func NewMutex() *Mutex {
	return &Mutex{
		mu: make(chan struct{}, 1), // unlocked by default
	}
}

var ErrFailToLock = errors.New("failed to lock")

func (m *Mutex) TryLock(ctx context.Context) error {
	select {
	case m.mu <- struct{}{}:
		return nil // locked
	case <-ctx.Done():
		return fmt.Errorf("%v: %w", ctx.Err(), ErrFailToLock)
	}
}

func (m *Mutex) Lock() {
	m.mu <- struct{}{}
}

func (m *Mutex) Unlock() {
	<-m.mu
}

// accountingPeer holds all in-memory accounting information for one peer.
type accountingPeer struct {
	lock                           *Mutex   // lock to be held during any accounting action for this peer
	reservedBalance                *big.Int // amount currently reserved for active peer interaction
	shadowReservedBalance          *big.Int // amount potentially to be debited for active peer interaction
	refreshReservedBalance         *big.Int // amount debt potentially decreased during an ongoing refreshment
	ghostBalance                   *big.Int // amount potentially could have been debited for but was not
	paymentThreshold               *big.Int // the threshold at which the peer expects us to pay
	earlyPayment                   *big.Int // individual early payment threshold calculated from from payment threshold and early payment percentage
	paymentThresholdForPeer        *big.Int // individual payment threshold at which the peer is expected to pay
	disconnectLimit                *big.Int // individual disconnect threshold calculated from tolerance and payment threshold for peer
	refreshTimestampMilliseconds   int64    // last time we attempted and succeeded time-based settlement
	refreshReceivedTimestamp       int64    // last time we accepted time-based settlement
	paymentOngoing                 bool     // indicate if we are currently settling with the peer
	refreshOngoing                 bool     // indicates if we are currently refreshing with the peer
	lastSettlementFailureTimestamp int64    // time of last unsuccessful attempt to issue a cheque
	connected                      bool     // indicates whether the peer is currently connected
	fullNode                       bool     // the peer connected as full node or light node
	totalDebtRepay                 *big.Int // since being connected, amount of cumulative debt settled by the peer
	thresholdGrowAt                *big.Int // cumulative debt to be settled by the peer in order to give threshold upgrade
}

// Accounting is the main implementation of the accounting interface.
type Accounting struct {
	// Mutex for accessing the accountingPeers map.
	accountingPeersMu sync.Mutex
	accountingPeers   map[string]*accountingPeer
	logger            log.Logger
	store             storage.StateStorer
	// The payment threshold in BZZ we communicate to our peers.
	paymentThreshold *big.Int
	// The amount in percent we let peers exceed the payment threshold before we
	// disconnect them.
	paymentTolerance int64
	// Start settling when reserve plus debt reaches this close to threshold in percent.
	earlyPayment int64
	// Limit to disconnect peer after going in debt over
	disconnectLimit *big.Int
	// function used for monetary settlement
	payFunction PayFunc
	// function used for time settlement
	refreshFunction RefreshFunc
	// allowance based on time used in pseudo settle
	refreshRate      *big.Int
	lightRefreshRate *big.Int
	// lower bound for the value of issued cheques
	minimumPayment      *big.Int
	pricing             pricing.Interface
	metrics             metrics
	wg                  sync.WaitGroup
	p2p                 p2p.Service
	timeNow             func() time.Time
	thresholdGrowStep   *big.Int
	thresholdGrowChange *big.Int
	// light node counterparts
	lightPaymentThreshold    *big.Int
	lightDisconnectLimit     *big.Int
	lightThresholdGrowStep   *big.Int
	lightThresholdGrowChange *big.Int
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
	// ErrOverRelease
	ErrOverRelease = errors.New("attempting to release more balance than was reserved for peer")
	// ErrEnforceRefresh
	ErrEnforceRefresh = errors.New("allowance expectation refused")
)

// NewAccounting creates a new Accounting instance with the provided options.
func NewAccounting(
	PaymentThreshold *big.Int,
	PaymentTolerance,
	EarlyPayment int64,
	logger log.Logger,
	Store storage.StateStorer,
	Pricing pricing.Interface,
	refreshRate *big.Int,
	lightFactor int64,
	p2pService p2p.Service,
) (*Accounting, error) {

	lightPaymentThreshold := new(big.Int).Div(PaymentThreshold, big.NewInt(lightFactor))
	lightRefreshRate := new(big.Int).Div(refreshRate, big.NewInt(lightFactor))
	return &Accounting{
		accountingPeers:          make(map[string]*accountingPeer),
		paymentThreshold:         new(big.Int).Set(PaymentThreshold),
		paymentTolerance:         PaymentTolerance,
		earlyPayment:             EarlyPayment,
		disconnectLimit:          percentOf(100+PaymentTolerance, PaymentThreshold),
		logger:                   logger.WithName(loggerName).Register(),
		store:                    Store,
		pricing:                  Pricing,
		metrics:                  newMetrics(),
		refreshRate:              new(big.Int).Set(refreshRate),
		lightRefreshRate:         new(big.Int).Div(refreshRate, big.NewInt(lightFactor)),
		timeNow:                  time.Now,
		minimumPayment:           new(big.Int).Div(refreshRate, big.NewInt(minimumPaymentDivisor)),
		p2p:                      p2pService,
		thresholdGrowChange:      new(big.Int).Mul(refreshRate, big.NewInt(linearCheckpointNumber)),
		thresholdGrowStep:        new(big.Int).Mul(refreshRate, big.NewInt(linearCheckpointStep)),
		lightPaymentThreshold:    new(big.Int).Set(lightPaymentThreshold),
		lightDisconnectLimit:     percentOf(100+PaymentTolerance, lightPaymentThreshold),
		lightThresholdGrowChange: new(big.Int).Mul(lightRefreshRate, big.NewInt(linearCheckpointNumber)),
		lightThresholdGrowStep:   new(big.Int).Mul(lightRefreshRate, big.NewInt(linearCheckpointStep)),
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

func (a *Accounting) PrepareCredit(ctx context.Context, peer swarm.Address, price uint64, originated bool) (Action, error) {
	loggerV1 := a.logger.V(1).Register()

	accountingPeer := a.getAccountingPeer(peer)

	if err := accountingPeer.lock.TryLock(ctx); err != nil {
		loggerV1.Debug("failed to acquire lock when preparing credit", "error", err)
		return nil, err
	}
	defer accountingPeer.lock.Unlock()

	if !accountingPeer.connected {
		return nil, errors.New("connection not initialized yet")
	}

	a.metrics.AccountingReserveCount.Inc()
	bigPrice := new(big.Int).SetUint64(price)

	threshold := accountingPeer.earlyPayment

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
			a.metrics.SettleErrorCount.Inc()
			return nil, fmt.Errorf("failed to settle with peer %v: %w", peer, err)
		}

		increasedExpectedDebt, _, err = a.getIncreasedExpectedDebt(peer, accountingPeer, bigPrice)
		if err != nil {
			return nil, err
		}
	}

	timeElapsedInSeconds := (a.timeNow().UnixMilli() - accountingPeer.refreshTimestampMilliseconds) / 1000
	if timeElapsedInSeconds > 1 {
		timeElapsedInSeconds = 1
	}

	refreshDue := new(big.Int).Mul(big.NewInt(timeElapsedInSeconds), a.refreshRate)
	overdraftLimit := new(big.Int).Add(accountingPeer.paymentThreshold, refreshDue)

	// if expectedDebt would still exceed the paymentThreshold at this point block this request
	// this can happen if there is a large number of concurrent requests to the same peer
	if increasedExpectedDebt.Cmp(overdraftLimit) > 0 {
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
	loggerV1 := c.accounting.logger.V(1).Register()

	c.accountingPeer.lock.Lock()
	defer c.accountingPeer.lock.Unlock()

	// debt if all reserved operations are successfully credited including debt created by surplus balance
	increasedExpectedDebt, currentBalance, err := c.accounting.getIncreasedExpectedDebt(c.peer, c.accountingPeer, c.price)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			return fmt.Errorf("failed to load balance: %w", err)
		}
	}

	// Calculate next balance by decreasing current balance with the price we credit
	nextBalance := new(big.Int).Sub(currentBalance, c.price)

	loggerV1.Debug("credit action apply", "crediting_peer_address", c.peer, "price", c.price, "new_balance", nextBalance)

	err = c.accounting.store.Put(peerBalanceKey(c.peer), nextBalance)
	if err != nil {
		return fmt.Errorf("failed to persist balance: %w", err)
	}

	c.accounting.metrics.TotalCreditedAmount.Add(float64(c.price.Int64()))
	c.accounting.metrics.CreditEventsCount.Inc()

	if c.price.Cmp(c.accountingPeer.reservedBalance) > 0 {
		c.accounting.logger.Error(ErrOverRelease, "for peer %v")
		c.accountingPeer.reservedBalance.SetUint64(0)
	} else {
		c.accountingPeer.reservedBalance.Sub(c.accountingPeer.reservedBalance, c.price)
	}

	c.applied = true

	if !c.originated {
		// debt if all reserved operations are successfully credited and all shadow reserved operations are debited including debt created by surplus balance
		// in other words this the debt the other node sees if everything pending is successful
		increasedExpectedDebtReduced := new(big.Int).Sub(increasedExpectedDebt, c.accountingPeer.shadowReservedBalance)
		if increasedExpectedDebtReduced.Cmp(c.accountingPeer.earlyPayment) > 0 {
			err = c.accounting.settle(c.peer, c.accountingPeer)
			if err != nil {
				c.accounting.logger.Error(err, "failed to settle with credited peer", c.peer)
			}
		}

		return nil
	}

	originBalance, err := c.accounting.OriginatedBalance(c.peer)
	if err != nil && !errors.Is(err, ErrPeerNoBalance) {
		return fmt.Errorf("failed to load originated balance: %w", err)
	}

	// Calculate next balance by decreasing current balance with the price we credit
	nextOriginBalance := new(big.Int).Sub(originBalance, c.price)

	loggerV1.Debug("credit action apply", "crediting_peer_address", c.peer, "price", c.price, "new_originated_balance", nextOriginBalance)

	zero := big.NewInt(0)
	// only consider negative balance for limiting originated balance
	if nextBalance.Cmp(zero) > 0 {
		nextBalance.Set(zero)
	}

	// If originated balance is more into the negative domain, set it to balance
	if nextOriginBalance.Cmp(nextBalance) < 0 {
		nextOriginBalance.Set(nextBalance)
		loggerV1.Debug("credit action apply; decreasing originated balance", "crediting_peer_address", c.peer, "current_balance", nextOriginBalance)
	}

	err = c.accounting.store.Put(originatedBalanceKey(c.peer), nextOriginBalance)
	if err != nil {
		return fmt.Errorf("failed to persist originated balance: %w", err)
	}

	c.accounting.metrics.TotalOriginatedCreditedAmount.Add(float64(c.price.Int64()))
	c.accounting.metrics.OriginatedCreditEventsCount.Inc()

	// debt if all reserved operations are successfully credited and all shadow reserved operations are debited including debt created by surplus balance
	// in other words this the debt the other node sees if everything pending is successful
	increasedExpectedDebtReduced := new(big.Int).Sub(increasedExpectedDebt, c.accountingPeer.shadowReservedBalance)
	if increasedExpectedDebtReduced.Cmp(c.accountingPeer.earlyPayment) > 0 {
		err = c.accounting.settle(c.peer, c.accountingPeer)
		if err != nil {
			c.accounting.logger.Error(err, "failed to settle with credited peer", c.peer)
		}
	}

	return nil
}

func (c *creditAction) Cleanup() {
	if c.applied {
		return
	}

	c.accountingPeer.lock.Lock()
	defer c.accountingPeer.lock.Unlock()

	if c.price.Cmp(c.accountingPeer.reservedBalance) > 0 {
		c.accounting.logger.Error(nil, "attempting to release more balance than was reserved for peer")
		c.accountingPeer.reservedBalance.SetUint64(0)
	} else {
		c.accountingPeer.reservedBalance.Sub(c.accountingPeer.reservedBalance, c.price)
	}
}

// Settle all debt with a peer. The lock on the accountingPeer must be held when
// called.
func (a *Accounting) settle(peer swarm.Address, balance *accountingPeer) error {
	now := a.timeNow()
	timeElapsedInMilliseconds := now.UnixMilli() - balance.refreshTimestampMilliseconds

	// get debt towards peer decreased by any amount that is to be debited soon
	paymentAmount, err := a.shadowBalance(peer, balance)
	if err != nil {
		return err
	}
	// Don't do anything if there is not enough actual debt
	// This might be the case if the peer owes us and the total reserve for a peer exceeds the payment threshold.
	// Minimum amount to trigger settlement for is 1 * refresh rate to avoid ineffective use of refreshments
	if paymentAmount.Cmp(a.refreshRate) >= 0 {
		// Only trigger refreshment if last refreshment finished at least 1000 milliseconds ago
		// This is to avoid a peer refusing refreshment because not enough time passed since last refreshment
		if timeElapsedInMilliseconds > 999 {
			if !balance.refreshOngoing {
				balance.refreshOngoing = true
				go a.refreshFunction(context.Background(), peer, paymentAmount)
			}
		}

		if a.payFunction != nil && !balance.paymentOngoing {
			// if a settlement failed recently, wait until failedSettlementInterval before trying again
			differenceInSeconds := now.Unix() - balance.lastSettlementFailureTimestamp
			if differenceInSeconds > failedSettlementInterval {
				// if there is no monetary settlement happening, check if there is something to settle
				// compute debt excluding debt created by incoming payments
				originatedBalance, err := a.OriginatedBalance(peer)
				if err != nil {
					if !errors.Is(err, ErrPeerNoBalance) {
						return fmt.Errorf("failed to load originated balance to settle: %w", err)
					}
				}

				paymentAmount := new(big.Int).Neg(originatedBalance)

				if paymentAmount.Cmp(a.minimumPayment) >= 0 {
					timeElapsedInSeconds := (a.timeNow().UnixMilli() - balance.refreshTimestampMilliseconds) / 1000
					refreshDue := new(big.Int).Mul(big.NewInt(timeElapsedInSeconds), a.refreshRate)
					currentBalance, err := a.Balance(peer)
					if err != nil && !errors.Is(err, ErrPeerNoBalance) {
						return fmt.Errorf("failed to load balance: %w", err)
					}

					debt := new(big.Int).Neg(currentBalance)
					decreasedDebt := new(big.Int).Sub(debt, refreshDue)
					expectedDecreasedDebt := new(big.Int).Sub(decreasedDebt, balance.shadowReservedBalance)

					if paymentAmount.Cmp(expectedDecreasedDebt) > 0 {
						paymentAmount.Set(expectedDecreasedDebt)
					}

					// if the remaining debt is still larger than some minimum amount, trigger monetary settlement
					if paymentAmount.Cmp(a.minimumPayment) >= 0 {
						balance.paymentOngoing = true
						// add settled amount to shadow reserve before sending it
						balance.shadowReservedBalance.Add(balance.shadowReservedBalance, paymentAmount)
						// if a refreshment is ongoing, add this amount sent to cumulative potential debt decrease during refreshment
						if balance.refreshOngoing {
							balance.refreshReservedBalance = new(big.Int).Add(balance.refreshReservedBalance, paymentAmount)
						}
						a.wg.Add(1)
						go a.payFunction(context.Background(), peer, paymentAmount)
					}
				}
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

// OriginatedBalance returns the current balance for the given peer.
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
			lock:                    NewMutex(),
			reservedBalance:         big.NewInt(0),
			refreshReservedBalance:  big.NewInt(0),
			shadowReservedBalance:   big.NewInt(0),
			ghostBalance:            big.NewInt(0),
			totalDebtRepay:          big.NewInt(0),
			paymentThreshold:        new(big.Int).Set(a.paymentThreshold),
			paymentThresholdForPeer: new(big.Int).Set(a.paymentThreshold),
			disconnectLimit:         new(big.Int).Set(a.disconnectLimit),
			thresholdGrowAt:         new(big.Int).Set(a.thresholdGrowStep),
			// initially assume the peer has the same threshold as us
			earlyPayment: percentOf(100-a.earlyPayment, a.paymentThreshold),
			connected:    false,
		}
		a.accountingPeers[peer.String()] = peerData
	}

	return peerData
}

// notifyPaymentThresholdUpgrade is used when cumulative debt settled by peer reaches current checkpoint,
// to set the next checkpoint and increase the payment threshold given by 1 * refreshment rate
// must be called under accountingPeer lock
func (a *Accounting) notifyPaymentThresholdUpgrade(peer swarm.Address, accountingPeer *accountingPeer) {

	// get appropriate linear growth limit based on whether the peer is a full node or a light node
	thresholdGrowChange := new(big.Int).Set(a.thresholdGrowChange)
	if !accountingPeer.fullNode {
		thresholdGrowChange.Set(a.lightThresholdGrowChange)
	}

	// if current checkpoint already passed linear growth limit, set next checkpoint exponentially
	if accountingPeer.thresholdGrowAt.Cmp(thresholdGrowChange) >= 0 {
		accountingPeer.thresholdGrowAt = new(big.Int).Mul(accountingPeer.thresholdGrowAt, big.NewInt(2))
	} else {
		// otherwise set next linear checkpoint
		if accountingPeer.fullNode {
			accountingPeer.thresholdGrowAt = new(big.Int).Add(accountingPeer.thresholdGrowAt, a.thresholdGrowStep)
		} else {
			accountingPeer.thresholdGrowAt = new(big.Int).Add(accountingPeer.thresholdGrowAt, a.lightThresholdGrowStep)
		}
	}

	// get appropriate refresh rate
	refreshRate := new(big.Int).Set(a.refreshRate)
	if !accountingPeer.fullNode {
		refreshRate = new(big.Int).Set(a.lightRefreshRate)
	}

	// increase given threshold by refresh rate
	accountingPeer.paymentThresholdForPeer = new(big.Int).Add(accountingPeer.paymentThresholdForPeer, refreshRate)
	// recalculate disconnectLimit for peer
	accountingPeer.disconnectLimit = percentOf(100+a.paymentTolerance, accountingPeer.paymentThresholdForPeer)

	// announce new payment threshold to peer
	err := a.pricing.AnnouncePaymentThreshold(context.Background(), peer, accountingPeer.paymentThresholdForPeer)
	if err != nil {
		a.logger.Error(err, "announcing increased payment threshold", "value", accountingPeer.paymentThresholdForPeer, "peer", peer)
	}
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

type PeerInfo struct {
	Balance                  *big.Int
	ConsumedBalance          *big.Int
	ThresholdReceived        *big.Int
	ThresholdGiven           *big.Int
	CurrentThresholdReceived *big.Int
	CurrentThresholdGiven    *big.Int
	SurplusBalance           *big.Int
	ReservedBalance          *big.Int
	ShadowReservedBalance    *big.Int
	GhostBalance             *big.Int
}

func (a *Accounting) PeerAccounting() (map[string]PeerInfo, error) {
	s := make(map[string]PeerInfo)

	a.accountingPeersMu.Lock()
	accountingPeersList := make(map[string]*accountingPeer)
	for peer, accountingPeer := range a.accountingPeers {
		accountingPeersList[peer] = accountingPeer
	}
	a.accountingPeersMu.Unlock()

	for peer, accountingPeer := range accountingPeersList {

		peerAddress := swarm.MustParseHexAddress(peer)

		balance, err := a.Balance(peerAddress)
		if errors.Is(err, ErrPeerNoBalance) {
			balance = big.NewInt(0)
		} else if err != nil {
			return nil, err
		}

		surplusBalance, err := a.SurplusBalance(peerAddress)
		if err != nil {
			return nil, err
		}

		accountingPeer.lock.Lock()

		t := a.timeNow()

		timeElapsedInSeconds := t.Unix() - accountingPeer.refreshReceivedTimestamp
		if timeElapsedInSeconds > 1 {
			timeElapsedInSeconds = 1
		}

		// get appropriate refresh rate
		refreshRate := new(big.Int).Set(a.refreshRate)
		if !accountingPeer.fullNode {
			refreshRate = new(big.Int).Set(a.lightRefreshRate)
		}

		refreshDue := new(big.Int).Mul(big.NewInt(timeElapsedInSeconds), refreshRate)
		currentThresholdGiven := new(big.Int).Add(accountingPeer.disconnectLimit, refreshDue)

		timeElapsedInSeconds = (t.UnixMilli() - accountingPeer.refreshTimestampMilliseconds) / 1000
		if timeElapsedInSeconds > 1 {
			timeElapsedInSeconds = 1
		}

		// get appropriate refresh rate
		refreshDue = new(big.Int).Mul(big.NewInt(timeElapsedInSeconds), a.refreshRate)
		currentThresholdReceived := new(big.Int).Add(accountingPeer.paymentThreshold, refreshDue)

		s[peer] = PeerInfo{
			Balance:                  new(big.Int).Sub(balance, surplusBalance),
			ConsumedBalance:          new(big.Int).Set(balance),
			ThresholdReceived:        new(big.Int).Set(accountingPeer.paymentThreshold),
			CurrentThresholdReceived: currentThresholdReceived,
			CurrentThresholdGiven:    currentThresholdGiven,
			ThresholdGiven:           new(big.Int).Set(accountingPeer.paymentThresholdForPeer),
			SurplusBalance:           new(big.Int).Set(surplusBalance),
			ReservedBalance:          new(big.Int).Set(accountingPeer.reservedBalance),
			ShadowReservedBalance:    new(big.Int).Set(accountingPeer.shadowReservedBalance),
			GhostBalance:             new(big.Int).Set(accountingPeer.ghostBalance),
		}
		accountingPeer.lock.Unlock()
	}

	return s, nil
}

// CompensatedBalances gets balances for all peers from store.
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
func (a *Accounting) shadowBalance(peer swarm.Address, accountingPeer *accountingPeer) (shadowBalance *big.Int, err error) {
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
	loggerV1 := a.logger.V(1).Register()

	defer a.wg.Done()
	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	accountingPeer.paymentOngoing = false
	// decrease shadow reserve by payment value
	accountingPeer.shadowReservedBalance.Sub(accountingPeer.shadowReservedBalance, amount)

	if receivedError != nil {
		accountingPeer.lastSettlementFailureTimestamp = a.timeNow().Unix()
		a.metrics.PaymentErrorCount.Inc()
		a.logger.Warning("payment failure", "error", receivedError)
		return
	}

	currentBalance, err := a.Balance(peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			a.logger.Error(err, "notify payment sent; failed to load balance")
			return
		}
	}

	// Get nextBalance by increasing current balance with price
	nextBalance := new(big.Int).Add(currentBalance, amount)

	loggerV1.Debug("registering payment sent", "peer", peer, "amount", amount, "new_balance", nextBalance)

	err = a.store.Put(peerBalanceKey(peer), nextBalance)
	if err != nil {
		a.logger.Error(err, "notify payment sent; failed to persist balance")
		return
	}

	err = a.decreaseOriginatedBalanceBy(peer, amount)
	if err != nil {
		a.logger.Warning("notify payment sent; failed to decrease originated balance", "error", err)
	}

}

// NotifyPaymentThreshold should be called to notify accounting of changes in the payment threshold
func (a *Accounting) NotifyPaymentThreshold(peer swarm.Address, paymentThreshold *big.Int) error {
	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	accountingPeer.paymentThreshold.Set(paymentThreshold)
	accountingPeer.earlyPayment.Set(percentOf(100-a.earlyPayment, paymentThreshold))
	return nil
}

// NotifyPaymentReceived is called by Settlement when we receive a payment.
func (a *Accounting) NotifyPaymentReceived(peer swarm.Address, amount *big.Int) error {
	loggerV1 := a.logger.V(1).Register()

	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	accountingPeer.totalDebtRepay = new(big.Int).Add(accountingPeer.totalDebtRepay, amount)

	if accountingPeer.totalDebtRepay.Cmp(accountingPeer.thresholdGrowAt) > 0 {
		a.notifyPaymentThresholdUpgrade(peer, accountingPeer)
	}

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

		loggerV1.Debug("surplus crediting peer", "peer_address", peer, "amount", amount, "new_balance", increasedSurplus)

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

	loggerV1.Debug("crediting peer", "peer_address", peer, "amount", amount, "new_balance", nextBalance)

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

		loggerV1.Debug("surplus crediting peer due to refreshment", "peer_address", peer, "amount", surplusGrowth, "new_balance", increasedSurplus)

		err = a.store.Put(peerSurplusBalanceKey(peer), increasedSurplus)
		if err != nil {
			return fmt.Errorf("failed to persist surplus balance: %w", err)
		}
	}

	return nil
}

// NotifyRefreshmentSent is called by pseudosettle when refreshment is done or failed
func (a *Accounting) NotifyRefreshmentSent(peer swarm.Address, attemptedAmount, amount *big.Int, timestamp int64, allegedInterval int64, receivedError error) {
	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	// conclude ongoing refreshment
	accountingPeer.refreshOngoing = false
	// save timestamp received in milliseconds of when the refreshment completed locally
	accountingPeer.refreshTimestampMilliseconds = timestamp

	// if specific error is received increment metrics
	if receivedError != nil {
		switch {
		case errors.Is(receivedError, pseudosettle.ErrRefreshmentAboveExpected):
			a.metrics.ErrRefreshmentAboveExpected.Inc()
		case errors.Is(receivedError, pseudosettle.ErrTimeOutOfSyncAlleged):
			a.metrics.ErrTimeOutOfSyncAlleged.Inc()
		case errors.Is(receivedError, pseudosettle.ErrTimeOutOfSyncRecent):
			a.metrics.ErrTimeOutOfSyncRecent.Inc()
		case errors.Is(receivedError, pseudosettle.ErrTimeOutOfSyncInterval):
			a.metrics.ErrTimeOutOfSyncInterval.Inc()
		}

		// if refreshment failed with connected peer, blocklist
		if !errors.Is(receivedError, p2p.ErrPeerNotFound) {
			a.metrics.AccountingDisconnectsEnforceRefreshCount.Inc()
			_ = a.blocklist(peer, 1, "failed to refresh")
		}
		a.logger.Error(receivedError, "notifyrefreshmentsent failed to refresh")
		return
	}

	// enforce allowance
	// calculate expectation decreased by any potential debt decreases occurred during the refreshment
	checkAllowance := new(big.Int).Sub(attemptedAmount, accountingPeer.refreshReservedBalance)

	// reset cumulative potential debt decrease during an ongoing refreshment as refreshment just completed
	accountingPeer.refreshReservedBalance.Set(big.NewInt(0))

	// dont expect higher amount accepted than attempted (sanity check)
	if checkAllowance.Cmp(attemptedAmount) > 0 {
		checkAllowance.Set(attemptedAmount)
	}

	// calculate time based allowance
	expectedAllowance := new(big.Int).Mul(big.NewInt(allegedInterval), a.refreshRate)
	// expect minimum of time based allowance and debt / attempted amount based expectation
	if expectedAllowance.Cmp(checkAllowance) > 0 {
		expectedAllowance = new(big.Int).Set(checkAllowance)
	}

	// compare received refreshment amount to expectation
	if expectedAllowance.Cmp(amount) > 0 {
		// if expectation is not met, blocklist peer
		a.logger.Error(ErrEnforceRefresh, "pseudosettle peer", peer, "accepted lower payment than expected")
		a.metrics.ErrRefreshmentBelowExpected.Inc()
		_ = a.blocklist(peer, 1, "failed to meet expectation for allowance")
		return
	}

	// update balance
	currentBalance, err := a.Balance(peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			a.logger.Error(err, "notifyrefreshmentsent failed to get balance")
			return
		}
	}

	newBalance := new(big.Int).Add(currentBalance, amount)

	err = a.store.Put(peerBalanceKey(peer), newBalance)
	if err != nil {
		a.logger.Error(err, "notifyrefreshmentsent failed to persist balance")
		return
	}

	// update originated balance
	err = a.decreaseOriginatedBalanceTo(peer, newBalance)
	if err != nil {
		a.logger.Warning("accounting: notifyrefreshmentsent failed to decrease originated balance: %v", err)
	}

}

// NotifyRefreshmentReceived is called by pseudosettle when we receive a time based settlement.
func (a *Accounting) NotifyRefreshmentReceived(peer swarm.Address, amount *big.Int, timestamp int64) error {
	loggerV1 := a.logger.V(1).Register()

	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	accountingPeer.totalDebtRepay = new(big.Int).Add(accountingPeer.totalDebtRepay, amount)

	if accountingPeer.totalDebtRepay.Cmp(accountingPeer.thresholdGrowAt) > 0 {
		a.notifyPaymentThresholdUpgrade(peer, accountingPeer)
	}

	currentBalance, err := a.Balance(peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			return err
		}
	}

	// Get nextBalance by increasing current balance with amount
	nextBalance := new(big.Int).Sub(currentBalance, amount)

	// We allow a refreshment to potentially put us into debt as it was previously negotiated and be limited to the peer's outstanding debt plus shadow reserve
	loggerV1.Debug("crediting peer", "peer_address", peer, "amount", amount, "new_balance", nextBalance)
	err = a.store.Put(peerBalanceKey(peer), nextBalance)
	if err != nil {
		return fmt.Errorf("failed to persist balance: %w", err)
	}

	accountingPeer.refreshReceivedTimestamp = timestamp

	return nil
}

// PrepareDebit prepares a debit operation by increasing the shadowReservedBalance
func (a *Accounting) PrepareDebit(ctx context.Context, peer swarm.Address, price uint64) (Action, error) {
	loggerV1 := a.logger.V(1).Register()

	accountingPeer := a.getAccountingPeer(peer)

	if err := accountingPeer.lock.TryLock(ctx); err != nil {
		loggerV1.Debug("prepare debit; failed to acquire lock", "error", err)
		return nil, err
	}

	defer accountingPeer.lock.Unlock()

	if !accountingPeer.connected {
		return nil, errors.New("connection not initialized yet")
	}

	bigPrice := new(big.Int).SetUint64(price)

	accountingPeer.shadowReservedBalance = new(big.Int).Add(accountingPeer.shadowReservedBalance, bigPrice)
	// if a refreshment is ongoing, add this amount to the potential debt decrease during an ongoing refreshment
	if accountingPeer.refreshOngoing {
		accountingPeer.refreshReservedBalance = new(big.Int).Add(accountingPeer.refreshReservedBalance, bigPrice)
	}

	return &debitAction{
		accounting:     a,
		price:          bigPrice,
		peer:           peer,
		accountingPeer: accountingPeer,
		applied:        false,
	}, nil
}

func (a *Accounting) increaseBalance(peer swarm.Address, _ *accountingPeer, price *big.Int) (*big.Int, error) {
	loggerV1 := a.logger.V(1).Register()

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
			loggerV1.Debug("surplus debiting peer", "peer_address", peer, "price", price, "new_balance", newSurplusBalance)

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
			return nil, errors.New("sanity check failed for partial debit after surplus balance drawn")
		}
		cost.Set(debitIncrease)

		// if we still have something to debit, than have run out of surplus balance,
		// let's store 0 as surplus balance
		loggerV1.Debug("surplus debiting peer", "peer_address", peer, "amount", debitIncrease, "new_balance", 0)

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

	loggerV1.Debug("debiting peer", "peer_address", peer, "price", price, "new_balance", nextBalance)

	err = a.store.Put(peerBalanceKey(peer), nextBalance)
	if err != nil {
		return nil, fmt.Errorf("failed to persist balance: %w", err)
	}

	err = a.decreaseOriginatedBalanceTo(peer, nextBalance)
	if err != nil {
		a.logger.Warning("increase balance; failed to decrease originated balance", "error", err)
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

	timeElapsedInSeconds := a.timeNow().Unix() - d.accountingPeer.refreshReceivedTimestamp
	if timeElapsedInSeconds > 1 {
		timeElapsedInSeconds = 1
	}

	// get appropriate refresh rate
	refreshRate := new(big.Int).Set(a.refreshRate)
	if !d.accountingPeer.fullNode {
		refreshRate = new(big.Int).Set(a.lightRefreshRate)
	}

	refreshDue := new(big.Int).Mul(big.NewInt(timeElapsedInSeconds), refreshRate)
	disconnectLimit := new(big.Int).Add(d.accountingPeer.disconnectLimit, refreshDue)

	if nextBalance.Cmp(disconnectLimit) >= 0 {
		// peer too much in debt
		a.metrics.AccountingDisconnectsOverdrawCount.Inc()

		disconnectFor, err := a.blocklistUntil(d.peer, 1)
		if err != nil {
			disconnectFor = 10
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
	if d.accountingPeer.ghostBalance.Cmp(d.accountingPeer.disconnectLimit) > 0 {
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

func (a *Accounting) Connect(peer swarm.Address, fullNode bool) {
	accountingPeer := a.getAccountingPeer(peer)
	zero := big.NewInt(0)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	paymentThreshold := new(big.Int).Set(a.paymentThreshold)
	thresholdGrowStep := new(big.Int).Set(a.thresholdGrowStep)
	disconnectLimit := new(big.Int).Set(a.disconnectLimit)

	if !fullNode {
		paymentThreshold.Set(a.lightPaymentThreshold)
		thresholdGrowStep.Set(a.lightThresholdGrowStep)
		disconnectLimit.Set(a.lightDisconnectLimit)
	}

	accountingPeer.connected = true
	accountingPeer.fullNode = fullNode
	accountingPeer.shadowReservedBalance.Set(zero)
	accountingPeer.ghostBalance.Set(zero)
	accountingPeer.reservedBalance.Set(zero)
	accountingPeer.refreshReservedBalance.Set(zero)
	accountingPeer.paymentThresholdForPeer.Set(paymentThreshold)
	accountingPeer.thresholdGrowAt.Set(thresholdGrowStep)
	accountingPeer.disconnectLimit.Set(disconnectLimit)

	err := a.store.Put(peerBalanceKey(peer), zero)
	if err != nil {
		a.logger.Error(err, "failed to persist balance")
	}

	err = a.store.Put(peerSurplusBalanceKey(peer), zero)
	if err != nil {
		a.logger.Error(err, "failed to persist surplus balance")
	}
}

// decreaseOriginatedBalanceTo decreases the originated balance to provided limit or 0 if limit is positive
func (a *Accounting) decreaseOriginatedBalanceTo(peer swarm.Address, limit *big.Int) error {
	loggerV1 := a.logger.V(1).Register()

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
		loggerV1.Debug("decreasing originated balance of peer", "peer_address", peer, "new_balance", toSet)
	}

	return nil
}

// decreaseOriginatedBalanceTo decreases the originated balance by provided amount even below 0
func (a *Accounting) decreaseOriginatedBalanceBy(peer swarm.Address, amount *big.Int) error {
	loggerV1 := a.logger.V(1).Register()

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
	loggerV1.Debug("decreasing originated balance of peer", "peer_address", peer, "amount", amount, "new_balance", newOriginatedBalance)

	return nil
}

func (a *Accounting) Disconnect(peer swarm.Address) {
	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	if accountingPeer.connected {
		disconnectFor, err := a.blocklistUntil(peer, 1)
		if err != nil {
			disconnectFor = int64(10)
		}
		accountingPeer.connected = false
		_ = a.p2p.Blocklist(peer, time.Duration(disconnectFor)*time.Second, "disconnected")
		a.metrics.AccountingDisconnectsReconnectCount.Inc()
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

func percentOf(percent int64, of *big.Int) *big.Int {
	return new(big.Int).Div(new(big.Int).Mul(of, big.NewInt(percent)), big.NewInt(100))
}
