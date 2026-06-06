// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redistribution

import "time"

// TxObserver receives transaction lifecycle events from the redistribution contract.
type TxObserver interface {
	OnTxCompleted(action string, sendTime time.Time, minedBlock uint64, err error)
}

// SetTxObserver attaches a transaction observer to a redistribution contract instance.
func SetTxObserver(c Contract, o TxObserver) {
	if sc, ok := c.(*contract); ok {
		sc.txObserver = o
	}
}
