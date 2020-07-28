// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accounting

import (
	"github.com/ethersphere/bee/pkg/swarm"
)

func BalanceKey(peer swarm.Address) string {
	return balanceKey(peer)
}
