// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/storer/internal/upload"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const subscribePushEventKey = "subscribe-push"

func (db *DB) SubscribePush(ctx context.Context) (<-chan swarm.Chunk, func()) {
	chunks := make(chan swarm.Chunk)

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
		defer close(chunks)
		for {

			err := upload.IteratePending(ctx, db.storage, func(chunk swarm.Chunk) (bool, error) {
				select {
				case chunks <- chunk:
					return false, nil
				case <-stopChan:
					// gracefully stop the iteration
					// on stop
					return true, nil
				case <-db.quit:
					return true, ErrDBQuit
				case <-ctx.Done():
					return true, ctx.Err()
				}
			})

			if err != nil {
				// if we get storage.ErrNotFound, it could happen that the previous
				// iteration happened on a snapshot that was not fully updated yet.
				// in this case, we wait for the next event to trigger the iteration
				// again. This trigger ensures that we perform the iteration on the
				// latest snapshot.
				db.logger.Error(err, "subscribe push: iterate error")
				select {
				case <-db.quit:
					return
				case <-ctx.Done():
					return
				case <-stopChan:
					return
				case <-time.After(time.Second):
				}
				db.events.Trigger(subscribePushEventKey)
			}

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

	return chunks, stop
}
