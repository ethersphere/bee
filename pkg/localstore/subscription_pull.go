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
	"errors"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/flipflop"
	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/syndtr/goleveldb/leveldb"
)

// SubscribePull returns a channel that provides chunk addresses and stored times from pull syncing index.
// Pull syncing index can be only subscribed to a particular proximity order bin. If since
// is not 0, the iteration will start from the since item (the item with binID == since). If until is not 0,
// only chunks stored up to this id will be sent to the channel, and the returned channel will be
// closed. The since-until interval is closed on since side, and closed on until side: [since,until]. Returned stop
// function will terminate current and further iterations without errors, and also close the returned channel.
// Make sure that you check the second returned parameter from the channel to stop iteration when its value
// is false.
func (db *DB) SubscribePull(ctx context.Context, bin uint8, since, until uint64) (c <-chan storage.Descriptor, closed <-chan struct{}, stop func()) {
	db.metrics.SubscribePull.Inc()

	chunkDescriptors := make(chan storage.Descriptor)

	in, out, clean := flipflop.NewFallingEdge(flipFlopBufferDuration, flipFlopWorstCaseDuration)

	db.pullTriggersMu.Lock()
	if _, ok := db.pullTriggers[bin]; !ok {
		db.pullTriggers[bin] = make([]chan<- struct{}, 0)
	}
	db.pullTriggers[bin] = append(db.pullTriggers[bin], in)
	db.pullTriggersMu.Unlock()

	// send signal for the initial iteration
	in <- struct{}{}

	stopChan := make(chan struct{})
	var stopChanOnce sync.Once

	// used to provide information from the iterator to
	// stop subscription when until chunk descriptor is reached
	var errStopSubscription = errors.New("stop subscription")

	db.subscritionsWG.Add(1)
	go func() {
		defer clean()
		defer db.subscritionsWG.Done()
		defer db.metrics.SubscribePullStop.Inc()
		// close the returned store.Descriptor channel at the end to
		// signal that the subscription is done
		defer close(chunkDescriptors)
		// sinceItem is the Item from which the next iteration
		// should start. The first iteration starts from the first Item.
		var sinceItem *shed.Item
		if since > 0 {
			sinceItem = &shed.Item{
				Address: db.addressInBin(bin).Bytes(),
				BinID:   since,
			}
		}
		first := true // first iteration flag for SkipStartFromItem
		for {
			select {
			case <-out:
				// iterate until:
				// - last index Item is reached
				// - subscription stop is called
				// - context is done
				db.metrics.SubscribePullIteration.Inc()

				iterStart := time.Now()
				var count int
				err := db.pullIndex.Iterate(func(item shed.Item) (stop bool, err error) {
					// until chunk descriptor is sent
					// break the iteration
					if until > 0 && item.BinID > until {
						return true, errStopSubscription
					}
					select {
					case chunkDescriptors <- storage.Descriptor{
						Address: swarm.NewAddress(item.Address),
						BinID:   item.BinID,
					}:
						if until > 0 && item.BinID == until {
							return true, errStopSubscription
						}

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
					// iterator call, skip it in this one, but not the item with
					// the provided since bin id as it should be sent to a channel
					SkipStartFromItem: !first,
					Prefix:            []byte{bin},
				})

				totalTimeMetric(db.metrics.TotalTimeSubscribePullIteration, iterStart)

				if err != nil {
					if err == errStopSubscription {
						// stop subscription without any errors
						// if until is reached
						return
					}
					db.metrics.SubscribePullIterationFailure.Inc()
					db.logger.Debugf("localstore pull subscription iteration: bin: %d, since: %d, until: %d: %v", bin, since, until, err)
					return
				}
				if count > 0 {
					first = false
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
					db.logger.Debugf("localstore pull subscription iteration: bin: %d, since: %d, until: %d: %v", bin, since, until, err)
				}
				return
			}
		}
	}()

	stop = func() {
		stopChanOnce.Do(func() {
			close(stopChan)
		})

		db.pullTriggersMu.Lock()
		defer db.pullTriggersMu.Unlock()

		for i, t := range db.pullTriggers[bin] {
			if t == in {
				db.pullTriggers[bin] = append(db.pullTriggers[bin][:i], db.pullTriggers[bin][i+1:]...)
				break
			}
		}
	}

	return chunkDescriptors, db.close, stop
}

// LastPullSubscriptionBinID returns chunk bin id of the latest Chunk
// in pull syncing index for a provided bin. If there are no chunks in
// that bin, 0 value is returned.
func (db *DB) LastPullSubscriptionBinID(bin uint8) (id uint64, err error) {
	db.metrics.LastPullSubscriptionBinID.Inc()

	item, err := db.pullIndex.Last([]byte{bin})
	if err != nil {
		if errors.Is(err, leveldb.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return item.BinID, nil
}

// triggerPullSubscriptions is used internally for starting iterations
// on Pull subscriptions for a particular bin. When new item with address
// that is in particular bin for DB's baseKey is added to pull index
// this function should be called.
func (db *DB) triggerPullSubscriptions(bin uint8) {
	db.pullTriggersMu.RLock()
	defer db.pullTriggersMu.RUnlock()

	triggers, ok := db.pullTriggers[bin]
	if !ok {
		return
	}
	for _, t := range triggers {
		select {
		case t <- struct{}{}:
		default:
		}
	}
}

// addressInBin returns an address that is in a specific
// proximity order bin from database base key.
func (db *DB) addressInBin(bin uint8) swarm.Address {
	addr := append([]byte(nil), db.baseKey...)
	b := bin / 8
	addr[b] = addr[b] ^ (1 << (7 - bin%8))
	return swarm.NewAddress(addr)
}
