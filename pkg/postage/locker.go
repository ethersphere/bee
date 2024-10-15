// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package postage

import (
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var StampLocker swarm.MultexLock

func init() {
	StampLocker = *swarm.NewMultexLock()
}

func LockKey(batchID []byte) string {
	return fmt.Sprintf("postage-%x", batchID)
}
