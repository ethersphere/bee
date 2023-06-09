// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"context"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/transaction"
)

var _ transaction.Backend = (*cachedBackend)(nil)

const defaultBlockNumberFetchInterval = time.Second * 20

type cachedBackend struct {
	transaction.Backend
	backend                  transaction.Backend
	nowFn                    func() time.Time
	blockNumber              uint64
	blockNumberLastFetchTime time.Time
	blockNumberFetchInterval time.Duration

	lock sync.Mutex
}

func New(backend transaction.Backend) *cachedBackend {
	return &cachedBackend{
		Backend:                  backend,
		backend:                  backend,
		nowFn:                    time.Now,
		blockNumberFetchInterval: defaultBlockNumberFetchInterval,
	}
}

func (b *cachedBackend) BlockNumber(ctx context.Context) (uint64, error) {
	b.lock.Lock()
	defer b.lock.Unlock()

	now := b.nowFn()
	if b.blockNumberLastFetchTime.Add(b.blockNumberFetchInterval).Before(now) {
		bno, err := b.backend.BlockNumber(ctx)
		if err != nil {
			return bno, err
		}

		b.blockNumber = bno
		b.blockNumberLastFetchTime = now
	}

	return b.blockNumber, nil
}
