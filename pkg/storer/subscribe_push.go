// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/storer/internal/upload"
	"github.com/ethersphere/bee/pkg/swarm"
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

			var count int
			start := time.Now()

			db.logger.Info("subscribe push: invoking upload.Iterate")
			err := upload.Iterate(ctx, db.repo, db.logger, func(chunk swarm.Chunk) (bool, error) {
				select {
				case chunks <- chunk:
					db.logger.Debug("subscribe push: upload.Iterate", "chunk", chunk.Address())
					count++
					return false, nil
				case <-stopChan:
					db.logger.Debug("subscribe push: upload.Iterate stopChan")
					// gracefully stop the iteration
					// on stop
					return true, nil
				case <-db.quit:
					db.logger.Debug("subscribe push: upload.Iterate db.quit")
					return true, ErrDBQuit
				case <-ctx.Done():
					db.logger.Error(ctx.Err(), "subscribe push: upload.Iterate ctx.Done (err?)")
					return true, ctx.Err()
				}
			})

			db.logger.Debug("subscribe push: upload.Iterate complete", "count", count, "elapsed", time.Since(start))

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
				db.logger.Error(err, "subscribe push: upload.Iterate error, retriggering")
				db.events.Trigger(subscribePushEventKey)
			}

			idle := time.Now()
			select {
			case <-db.quit:
				db.logger.Debug("subscribe push: upload.Iterate eol db.quit", "idle", time.Since(idle))
				return
			case <-ctx.Done():
				db.logger.Debug("subscribe push: upload.Iterate eol ctx.Done", "idle", time.Since(idle))
				return
			case <-stopChan:
				db.logger.Debug("subscribe push: upload.Iterate eol stopChan", "idle", time.Since(idle))
				return
			case <-trigger:
				db.logger.Debug("subscribe push: upload.Iterate eol triggered", "idle", time.Since(idle))
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
