// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const subscribePushEventKey = "subscribe-push"

func (db *DB) SubscribePush(ctx context.Context) (<-chan swarm.Chunk, func()) {
	cc := make(chan swarm.Chunk)

	var (
		stopChan     = make(chan struct{})
		stopChanOnce sync.Once
	)

	db.subscriptionsWG.Add(1)
	go func() {
		defer db.subscriptionsWG.Done()

		trigger, unsub := db.events.Subscribe(subscribePushEventKey)
		defer unsub()

		// close the returned chunkInfo channel at the end to
		// signal that the subscription is done
		defer close(cc)

		// drain fetches batch size items at a time from the pending list of chunks until the entire list is drained.
		drain := func() {

			const batchSize = 1000

			for {
				chunks := make([]swarm.Chunk, 0, batchSize)

				err := db.IteratePendingUpload(ctx, db.storage, func(chunk swarm.Chunk) (bool, error) {
					chunks = append(chunks, chunk)
					if len(chunks) == batchSize {
						return true, nil
					}
					return false, nil
				})

				if err != nil {
					db.logger.Error(err, "subscribe push: iterate error")
				}

				for _, chunk := range chunks {
					select {
					case cc <- chunk:
					case <-stopChan:
						return
					case <-db.quit:
						return
					case <-ctx.Done():
						return
					}
				}

				if len(chunks) < batchSize { // no more work
					return
				}
			}
		}

		for {

			drain()

			select {
			case <-db.quit:
				return
			case <-ctx.Done():
				return
			case <-stopChan:
				return
			case <-trigger:
				// wait for the next event
			}
		}
	}()

	stop := func() {
		stopChanOnce.Do(func() {
			close(stopChan)
		})
	}

	return cc, stop
}
