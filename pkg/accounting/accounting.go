package accounting

import (
	"errors"
	"sync"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/swarm"
)

var _ Interface = (*Accounting)(nil)

// Interface is the main interface for Accounting
type Interface interface {
	// Reserve reserves a portion of the balance for peer
	Reserve(peer swarm.Address, price uint64) error
	// Release releases reserved funds
	Release(peer swarm.Address, price uint64)
	// Apply a balance change to the peers balance
	Add(peer swarm.Address, price int64) error
}

type PeerBalance struct {
	lock     sync.Mutex
	balance  int64 // positive amount is the nodes debt with us
	reserved uint64
}

type Options struct {
	DisconnectThreshold uint64
	PaymentThreshold    uint64
	Logger              logging.Logger
}

type Accounting struct {
	balancesMu sync.Mutex
	balances   map[string]*PeerBalance
	logger     logging.Logger

	disconnectThreshold uint64
	paymentThreshold    uint64
}

var (
	DisconnectError   = errors.New("DisconnectError")
	CannotAffordError = errors.New("CannotAffordError")
)

func NewAccounting(o Options) *Accounting {
	return &Accounting{
		balances: make(map[string]*PeerBalance),

		paymentThreshold:    o.PaymentThreshold,
		disconnectThreshold: o.DisconnectThreshold,
		logger:              o.Logger,
	}
}

func (a *Accounting) Reserve(peer swarm.Address, price uint64) error {
	balance := a.getPeerBalance(peer)
	balance.lock.Lock()
	defer balance.lock.Unlock()

	if balance.balance-int64(balance.reserved+price) < -int64(a.disconnectThreshold) {
		return CannotAffordError
	}

	balance.reserved += price

	return nil
}

func (a *Accounting) Release(peer swarm.Address, price uint64) {
	balance := a.getPeerBalance(peer)
	balance.lock.Lock()
	defer balance.lock.Unlock()

	if price > balance.reserved {
		a.logger.Error("Releasing more balance than was reserved for peer")
		balance.reserved = 0
	} else {
		balance.reserved -= price
	}
}

func (a *Accounting) Add(peer swarm.Address, price int64) error {
	balance := a.getPeerBalance(peer)
	balance.lock.Lock()
	defer balance.lock.Unlock()

	if price > 0 {
		// we gain balannce ith the peer
		if balance.balance+price > int64(a.disconnectThreshold) {
			// peer to much in debt
			return DisconnectError
		}
	}

	balance.balance += price
	// TODO: save to state store

	// TODO: try to initiate payment if payment threshold is reached
	// if balance.balance < -int64(a.paymentThreshold) { }

	return nil
}

func (a *Accounting) GetPeerBalance(peer swarm.Address) int64 {
	return a.getPeerBalance(peer).balance
}

func (a *Accounting) getPeerBalance(peer swarm.Address) *PeerBalance {
	a.balancesMu.Lock()
	defer a.balancesMu.Unlock()

	peerInfo, ok := a.balances[peer.String()]
	if !ok {
		// TODO: try loading from state store
		peerInfo = &PeerBalance{
			balance:  0,
			reserved: 0,
		}
		a.balances[peer.String()] = peerInfo
		return peerInfo
	}

	return peerInfo
}
