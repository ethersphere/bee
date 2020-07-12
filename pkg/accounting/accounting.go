package accounting

import (
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var _ Interface = (*Accounting)(nil)

// Interface is the main interface for Accounting
type Interface interface {
	// Reserve reserves a portion of the balance for peer
	Reserve(peer swarm.Address, price uint64) error
	// Release releases reserved funds
	Release(peer swarm.Address, price uint64)
	// Credit the peer with the given price
	Credit(peer swarm.Address, price uint64) error
	// Debit the peer with the given price
	Debit(peer swarm.Address, price uint64) error
}

type PeerBalance struct {
	lock     sync.Mutex
	balance  int64  // amount that the peer owes us if positive, our debt if negative
	reserved uint64 // amount currently reserved for active peer interaction
}

type Options struct {
	DisconnectThreshold uint64
	PaymentThreshold    uint64
	Logger              logging.Logger
	Store               storage.StateStorer
}

type Accounting struct {
	balancesMu sync.Mutex
	balances   map[string]*PeerBalance
	logger     logging.Logger
	store      storage.StateStorer

	disconnectThreshold uint64
	paymentThreshold    uint64
}

func NewAccounting(o Options) *Accounting {
	return &Accounting{
		balances: make(map[string]*PeerBalance),

		paymentThreshold:    o.PaymentThreshold,
		disconnectThreshold: o.DisconnectThreshold,
		logger:              o.Logger,
		store:               o.Store,
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

	if balance.balance-int64(balance.reserved+price) < -int64(a.disconnectThreshold) {
		return fmt.Errorf("cannot afford operation with peer %v", peer.String())
	}

	balance.reserved += price

	return nil
}

// Release releases reserved funds
func (a *Accounting) Release(peer swarm.Address, price uint64) {
	balance, err := a.getPeerBalance(peer)
	if err != nil {
		a.logger.Error("Releasing balance for peer that is not loaded")
		return
	}

	balance.lock.Lock()
	defer balance.lock.Unlock()

	if price > balance.reserved {
		a.logger.Error("Releasing more balance than was reserved for peer")
		balance.reserved = 0
	} else {
		balance.reserved -= price
	}
}

// Debit the peer with the given price
func (a *Accounting) Debit(peer swarm.Address, price uint64) error {
	balance, err := a.getPeerBalance(peer)
	if err != nil {
		return err
	}

	balance.lock.Lock()
	defer balance.lock.Unlock()

	nextBalance := balance.balance + int64(price)

	a.logger.Tracef("debiting peer %v with price %d, new balance is %d", peer, price, nextBalance)

	if nextBalance >= int64(a.disconnectThreshold) {
		// peer to much in debt
		return p2p.NewDisconnectError(fmt.Errorf("disconnect threshold exceeded for peer %s", peer.String()))
	}

	err = a.store.Put(a.balanceKey(peer), nextBalance)
	if err != nil {
		return err
	}

	balance.balance = nextBalance
	return nil
}

// Credit the peer with the given price
func (a *Accounting) Credit(peer swarm.Address, price uint64) error {
	balance, err := a.getPeerBalance(peer)
	if err != nil {
		return err
	}

	balance.lock.Lock()
	defer balance.lock.Unlock()

	nextBalance := balance.balance - int64(price)

	a.logger.Infof("crediting peer %v with price %d, new balance is %d", peer, price, nextBalance)

	err = a.store.Put(a.balanceKey(peer), nextBalance)
	if err != nil {
		return err
	}

	balance.balance = nextBalance

	// TODO: try to initiate payment if payment threshold is reached
	// if balance.balance < -int64(a.paymentThreshold) { }

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
func (a *Accounting) balanceKey(peer swarm.Address) string {
	return fmt.Sprintf("balance_%s", peer.String())
}

func (a *Accounting) getPeerBalance(peer swarm.Address) (*PeerBalance, error) {
	a.balancesMu.Lock()
	defer a.balancesMu.Unlock()

	peerInfo, ok := a.balances[peer.String()]
	if !ok {
		var balance int64
		err := a.store.Get(a.balanceKey(peer), &balance)
		if err == nil {
			peerInfo = &PeerBalance{
				balance:  balance,
				reserved: 0,
			}
		} else if err == storage.ErrNotFound {
			peerInfo = &PeerBalance{
				balance:  0,
				reserved: 0,
			}
		} else {
			return nil, err
		}

		a.balances[peer.String()] = peerInfo
		return peerInfo, nil
	}

	return peerInfo, nil
}
