// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package transaction

import (
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var transactionLocker swarm.MultexLock

func init() {
	transactionLocker = *swarm.NewMultexLock()
}

func LockKey(addr swarm.Address) string {
	return fmt.Sprintf("internal-transaction-%x", addr)
}
