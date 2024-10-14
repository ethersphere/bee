// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package pusher

import (
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var PusherLocker swarm.MultexLock

func init() {
	PusherLocker = *swarm.NewMultexLock()
}

func lockKey(addr swarm.Address) string {
	bucket := postage.ToBucket(16, addr)
	return fmt.Sprintf("pusher-%x", bucket)
}
