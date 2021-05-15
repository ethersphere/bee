// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package settlement

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	ErrPeerNoSettlements = errors.New("no settlements for peer")
)

// Interface is the interface used by Accounting to trigger settlement
type Interface interface {
	// TotalSent returns the total amount sent to a peer
	TotalSent(peer swarm.Address) (totalSent *big.Int, err error)
	// TotalReceived returns the total amount received from a peer
	TotalReceived(peer swarm.Address) (totalSent *big.Int, err error)
	// SettlementsSent returns sent settlements for each individual known peer
	SettlementsSent() (map[string]*big.Int, error)
	// SettlementsReceived returns received settlements for each individual known peer
	SettlementsReceived() (map[string]*big.Int, error)
}

type AccountingAPI interface {
	PeerDebt(peer swarm.Address) (*big.Int, error)
	NotifyPaymentReceived(peer swarm.Address, amount *big.Int) error
	NotifyPaymentSent(peer swarm.Address, amount *big.Int, receivedError error)
	NotifyRefreshmentReceived(peer swarm.Address, amount *big.Int) error

	Reserve(ctx context.Context, peer swarm.Address, price uint64) error
	Release(peer swarm.Address, price uint64)
}
