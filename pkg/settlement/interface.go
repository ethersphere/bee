// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package settlement

import (
	"context"

	"github.com/ethersphere/bee/pkg/swarm"
)

// Interface is the interface used by Accounting to trigger settlement
type Interface interface {
	// Pay initiates a payment to the given peer
	// It should return without error it is likely that the payment worked
	Pay(ctx context.Context, peer swarm.Address, amount uint64) error
	// TotalSent returns the total amount sent to a peer
	TotalSent(peer swarm.Address) (totalSent uint64, err error)
	// TotalReceived returns the total amount received from a peer
	TotalReceived(peer swarm.Address) (totalSent uint64, err error)
	// SettlementsSent returns sent settlements for each individual known peer
	SettlementsSent() (map[string]uint64, error)
	// SettlementsReceived returns received settlements for each individual known peer
	SettlementsReceived() (map[string]uint64, error)
}

// PaymentObserver is the interface Settlement uses to notify other components of an incoming payment
type PaymentObserver interface {
	// NotifyPayment is called when a payment from peer was successfully received
	NotifyPayment(peer swarm.Address, amount uint64) error
}
