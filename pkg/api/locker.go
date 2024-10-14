// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package api

import (
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var ApiLocker swarm.MultexLock

func init() {
	ApiLocker = *swarm.NewMultexLock()
}

func lockKey(socAddr swarm.Address) string {
	bucket := postage.ToBucket(16, socAddr)
	return fmt.Sprintf("api-%x", bucket)
}
