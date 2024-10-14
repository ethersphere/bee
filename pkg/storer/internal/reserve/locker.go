// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package reserve

import (
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var reserveLocker swarm.MultexLock

func init() {
	reserveLocker = *swarm.NewMultexLock()
}

func LockKey(bucketId uint32) string {
	return fmt.Sprintf("internal-reserve-%x", bucketId)
}
