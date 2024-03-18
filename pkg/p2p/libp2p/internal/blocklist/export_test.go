// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package blocklist

import (
	"github.com/ethersphere/bee/v2/pkg/storage"
)

func NewBlocklistWithCurrentTimeFn(store storage.StateStorer, currentTimeFn currentTimeFn) *Blocklist {
	return &Blocklist{
		store:         store,
		currentTimeFn: currentTimeFn,
	}
}
