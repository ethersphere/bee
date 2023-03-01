// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"sync"

	"github.com/ethersphere/bee/pkg/localstorev2/internal/upload"
	"github.com/ethersphere/bee/pkg/swarm"
)

const subscribePushEventKey = "subscribe-push"

func (db *DB) SubscribePush(ctx context.Context) (chunks chan swarm.Chunk, stop func()) {
	chunks = make(chan swarm.Chunk)

	trigger, unsub := db.events.Subscribe(subscribePushEventKey)
	defer unsub()

	// send signal for the initial iteration
	db.events.Trigger(subscribePushEventKey)

	stopChan := make(chan struct{})
	var stopChanOnce sync.Once

	var sinceItem swarm.Chunk

	db.subscriptionsWG.Add(1)
	go func() {
		defer db.subscriptionsWG.Done()
		// close the returned chunkInfo channel at the end to
		// signal that the subscription is done
		defer close(chunks)
		// sinceItem is the Item from which the next iteration
		// should start. The first iteration starts from the first Item.
		for {

			var count int

			err := upload.Iterate(ctx, db.repo, sinceItem, func(chunk swarm.Chunk) (bool, error) {

				if db.isDirty(uint64(chunk.TagID())) {
					return true, nil
				}

				select {
				case chunks <- chunk:
					count++
					// set next iteration start item
					// when its chunk is successfully sent to channel
					sinceItem = chunk

					return false, nil
				case <-stopChan:
					// gracefully stop the iteration
					// on stop
					return true, nil
				case <-db.quit:
					return true, errDBQuit
				case <-ctx.Done():
					return true, ctx.Err()
				}
			})

			if err != nil {
				return
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

	stop = func() {
		stopChanOnce.Do(func() {
			close(stopChan)
		})
	}

	return chunks, stop
}

func (db *DB) isDirty(tag uint64) bool {
	db.dirtyTagsMu.RLock()
	defer db.dirtyTagsMu.RUnlock()

	for _, dirtyTag := range db.dirtyTags {
		if dirtyTag == tag {
			return true
		}
	}
	return false
}
