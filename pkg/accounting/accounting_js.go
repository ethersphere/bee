//go:build js
// +build js

package accounting

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/pricing"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

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

func (a *Accounting) PrepareCredit(ctx context.Context, peer swarm.Address, price uint64, originated bool) (Action, error) {

	accountingPeer := a.getAccountingPeer(peer)

	if err := accountingPeer.lock.TryLock(ctx); err != nil {
		a.logger.Debug("failed to acquire lock when preparing credit", "error", err)
		return nil, err
	}
	defer accountingPeer.lock.Unlock()

	if !accountingPeer.connected {
		return nil, errors.New("connection not initialized yet")
	}

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
	loggerV2 := c.accounting.logger.V(2).Register()

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

	loggerV2.Debug("credit action apply", "crediting_peer_address", c.peer, "price", c.price, "new_balance", nextBalance)

	err = c.accounting.store.Put(peerBalanceKey(c.peer), nextBalance)
	if err != nil {
		return fmt.Errorf("failed to persist balance: %w", err)
	}

	if c.price.Cmp(c.accountingPeer.reservedBalance) > 0 {
		c.accounting.logger.Error(nil, "attempting to release more balance than was reserved for peer", "peer_address", c.peer)
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
				c.accounting.logger.Error(err, "failed to settle with credited peer", "peer_address", c.peer)
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

	loggerV2.Debug("credit action apply", "crediting_peer_address", c.peer, "price", c.price, "new_originated_balance", nextOriginBalance)

	zero := big.NewInt(0)
	// only consider negative balance for limiting originated balance
	if nextBalance.Cmp(zero) > 0 {
		nextBalance.Set(zero)
	}

	// If originated balance is more into the negative domain, set it to balance
	if nextOriginBalance.Cmp(nextBalance) < 0 {
		nextOriginBalance.Set(nextBalance)
		loggerV2.Debug("credit action apply; decreasing originated balance", "crediting_peer_address", c.peer, "current_balance", nextOriginBalance)
	}

	err = c.accounting.store.Put(originatedBalanceKey(c.peer), nextOriginBalance)
	if err != nil {
		return fmt.Errorf("failed to persist originated balance: %w", err)
	}

	// debt if all reserved operations are successfully credited and all shadow reserved operations are debited including debt created by surplus balance
	// in other words this the debt the other node sees if everything pending is successful
	increasedExpectedDebtReduced := new(big.Int).Sub(increasedExpectedDebt, c.accountingPeer.shadowReservedBalance)
	if increasedExpectedDebtReduced.Cmp(c.accountingPeer.earlyPayment) > 0 {
		err = c.accounting.settle(c.peer, c.accountingPeer)
		if err != nil {
			c.accounting.logger.Error(err, "failed to settle with credited peer", "peer_address", c.peer)
		}
	}

	return nil
}

// NotifyPaymentSent is triggered by async monetary settlement to update our balance and remove it's price from the shadow reserve
func (a *Accounting) NotifyPaymentSent(peer swarm.Address, amount *big.Int, receivedError error) {
	loggerV2 := a.logger.V(2).Register()

	defer a.wg.Done()
	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	accountingPeer.paymentOngoing = false
	// decrease shadow reserve by payment value
	accountingPeer.shadowReservedBalance.Sub(accountingPeer.shadowReservedBalance, amount)

	if receivedError != nil {
		accountingPeer.lastSettlementFailureTimestamp = a.timeNow().Unix()

		a.logger.Warning("payment failure", "error", receivedError)
		return
	}

	currentBalance, err := a.Balance(peer)
	if err != nil {
		if !errors.Is(err, ErrPeerNoBalance) {
			a.logger.Error(err, "notify payment sent; failed to persist balance")
			return
		}
	}

	// Get nextBalance by increasing current balance with price
	nextBalance := new(big.Int).Add(currentBalance, amount)

	loggerV2.Debug("registering payment sent", "peer_address", peer, "amount", amount, "new_balance", nextBalance)

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

		// if refreshment failed with connected peer, blocklist
		if !errors.Is(receivedError, p2p.ErrPeerNotFound) {
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
		a.logger.Error(nil, "accepted lower payment than expected", "pseudosettle peer", peer)

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
		a.logger.Warning("accounting: notifyrefreshmentsent failed to decrease originated balance", "error", err)
	}

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

		disconnectFor, err := a.blocklistUntil(d.peer, 1)
		if err != nil {
			disconnectFor = 10
		}
		return p2p.NewBlockPeerError(time.Duration(disconnectFor)*time.Second, ErrDisconnectThresholdExceeded)

	}

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
		_ = a.p2p.Blocklist(peer, time.Duration(disconnectFor)*time.Second, "accounting disconnect")

	}
}
