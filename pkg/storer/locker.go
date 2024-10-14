// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package storer

import (
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var storerLocker, reserveLocker swarm.MultexLock

func init() {
	storerLocker = *swarm.NewMultexLock()
	reserveLocker = *swarm.NewMultexLock()
}

func LockKey(bucketId uint32) string {
	return fmt.Sprintf("storer-%x", bucketId)
}
