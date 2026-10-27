// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"errors"

	"github.com/ethersphere/bee/v2/pkg/bmt"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/events"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/syndtr/goleveldb/leveldb/util"
)

func (db *DB) Reserve() *reserve.Reserve {
	return db.reserve
}

func (db *DB) Events() *events.Subscriber {
	return db.events
}

func ReplaceSharkyShardLimit(val int) {
	sharkyNoOfShards = val
}

func (db *DB) WaitForBgCacheWorkers() (unblock func()) {
	for range defaultBgCacheWorkers {
		db.cacheLimiter.sem <- struct{}{}
	}
	return func() {
		for range defaultBgCacheWorkers {
			<-db.cacheLimiter.sem
		}
	}
}

func DefaultOptions() *Options {
	return defaultOptions()
}

// TransformedAddress exposes the sampler's per-chunk hashing so it can be
// benchmarked on its own.
//
// The exported signature takes a swarm.Chunk and is deliberately held stable
// even where transformedAddress itself does not, so that the same benchmark
// source can be run against branches that shape the internal function
// differently. Only this shim changes between them.
func TransformedAddress(hasher bmt.Hasher, ch swarm.Chunk, chType swarm.ChunkType) (swarm.Address, error) {
	return transformedAddress(hasher, ch.Address(), ch.Data(), chType)
}

// AddrIndexFills returns how many address index fills New ran.
func (db *DB) AddrIndexFills() int {
	return db.addrIndex.fills
}

// WaitAddrIndexDrop waits for the background drop of a disabled address
// index, if New started one.
func (db *DB) WaitAddrIndexDrop() {
	if db.addrIndex.dropDone != nil {
		<-db.addrIndex.dropDone
	}
}

// AddrIndexDropStarted reports whether New started a background drop of the
// address index.
func (db *DB) AddrIndexDropStarted() bool {
	return db.addrIndex.dropDone != nil
}

// CountWithinRadius runs the reserve worker's count of the reserve chunks
// within the storage radius, which also evicts the chunks of batches the
// batch store does not know.
func (db *DB) CountWithinRadius(ctx context.Context) (int, error) {
	return db.countWithinRadius(ctx)
}

// SetReserveTestOptions sets options New passes to the reserve.
func SetReserveTestOptions(o *Options, opts ...reserve.Option) {
	o.reserveTestOptions = opts
}

// SetAddrIndexCompactHook sets a function New's DB calls before each
// compaction of the address index key range.
func SetAddrIndexCompactHook(o *Options, hook func()) {
	o.addrIndexCompactHook = hook
}

// AddrIndexRangeSize returns the approximate size of the address index key
// range in the leveldb tables (leveldb SizeOf; the memtable is not counted).
func (db *DB) AddrIndexRangeSize() (int64, error) {
	ldb, ok := db.pinIntegrity.Store.(leveldbstore.Storer)
	if !ok {
		return 0, errors.New("not a leveldb store")
	}
	sizes, err := ldb.DB().SizeOf([]util.Range{reserve.AddrIndexKeyRange()})
	if err != nil {
		return 0, err
	}
	return sizes.Sum(), nil
}
