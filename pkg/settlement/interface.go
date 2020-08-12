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
}

// PaymentObserver is the interface Settlement uses to notify other components of an incoming payment
type PaymentObserver interface {
	// NotifyPayment is called when a payment from peer was successfully received
	NotifyPayment(peer swarm.Address, amount uint64) error
}
