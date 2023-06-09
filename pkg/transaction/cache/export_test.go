// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"time"

	"github.com/ethersphere/bee/pkg/transaction"
)

const DefaultBlockNumberFetchInterval = defaultBlockNumberFetchInterval

func NewWithOptions(
	backend transaction.Backend,
	nowFn func() time.Time,
) transaction.Backend {
	b := New(backend)
	b.nowFn = nowFn

	return b
}
