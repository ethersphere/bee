// Copyright 2019 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package localstore

import (
	"context"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/flipflop"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/swarm"
)

// SubscribePush returns a channel that provides storage chunks with ordering from push syncing index.
// Returned stop function will terminate current and further iterations, and also it will close
// the returned channel without any errors. Make sure that you check the second returned parameter
// from the channel to stop iteration when its value is false.
func (db *DB) SubscribePush(ctx context.Context) (c <-chan swarm.Chunk, stop func()) {
	db.metrics.SubscribePush.Inc()

	chunks := make(chan swarm.Chunk)
	in, out, clean := flipflop.NewFallingEdge(flipFlopBufferDuration, flipFlopWorstCaseDuration)

	db.pushTriggersMu.Lock()
	db.pushTriggers = append(db.pushTriggers, in)
	db.pushTriggersMu.Unlock()

	// send signal for the initial iteration
	in <- struct{}{}

	stopChan := make(chan struct{})
	var stopChanOnce sync.Once

	db.subscritionsWG.Add(1)
	go func() {
		defer clean()
		defer db.subscritionsWG.Done()
		defer db.metrics.SubscribePushIterationDone.Inc()
		// close the returned chunkInfo channel at the end to
		// signal that the subscription is done
		defer close(chunks)
		// sinceItem is the Item from which the next iteration
		// should start. The first iteration starts from the first Item.
		var sinceItem *shed.Item
		for {
			select {
			case <-out:
				// iterate until:
				// - last index Item is reached
				// - subscription stop is called
				// - context is done.met
				db.metrics.SubscribePushIteration.Inc()

				iterStart := time.Now()
				var count int
				err := db.pushIndex.Iterate(func(item shed.Item) (stop bool, err error) {
					// get chunk data
					dataItem, err := db.retrievalDataIndex.Get(item)
					if err != nil {
						return true, err
					}

					stamp := postage.NewStamp(dataItem.BatchID, dataItem.Index, dataItem.Timestamp, dataItem.Sig)
					select {
					case chunks <- swarm.NewChunk(swarm.NewAddress(dataItem.Address), dataItem.Data).WithTagID(item.Tag).WithStamp(stamp):
						count++
						// set next iteration start item
						// when its chunk is successfully sent to channel
						sinceItem = &item
						return false, nil
					case <-stopChan:
						// gracefully stop the iteration
						// on stop
						return true, nil
					case <-db.close:
						// gracefully stop the iteration
						// on database close
						return true, nil
					case <-ctx.Done():
						return true, ctx.Err()
					}
				}, &shed.IterateOptions{
					StartFrom: sinceItem,
					// sinceItem was sent as the last Address in the previous
					// iterator call, skip it in this one
					SkipStartFromItem: true,
				})

				totalTimeMetric(db.metrics.TotalTimeSubscribePushIteration, iterStart)

				if err != nil {
					db.metrics.SubscribePushIterationFailure.Inc()
					db.logger.Debugf("localstore push subscription iteration: %v", err)
					return
				}

			case <-stopChan:
				// terminate the subscription
				// on stop
				return
			case <-db.close:
				// terminate the subscription
				// on database close
				return
			case <-ctx.Done():
				err := ctx.Err()
				if err != nil {
					db.logger.Debugf("localstore push subscription iteration: %v", err)
				}
				return
			}
		}
	}()

	stop = func() {
		stopChanOnce.Do(func() {
			close(stopChan)
		})

		db.pushTriggersMu.Lock()
		defer db.pushTriggersMu.Unlock()

		for i, t := range db.pushTriggers {
			if t == in {
				db.pushTriggers = append(db.pushTriggers[:i], db.pushTriggers[i+1:]...)
				break
			}
		}
	}

	return chunks, stop
}

// triggerPushSubscriptions is used internally for starting iterations
// on Push subscriptions. Whenever new item is added to the push index,
// this function should be called.
func (db *DB) triggerPushSubscriptions() {
	db.pushTriggersMu.RLock()
	defer db.pushTriggersMu.RUnlock()

	for _, t := range db.pushTriggers {
		select {
		case t <- struct{}{}:
		default:
		}
	}
}
