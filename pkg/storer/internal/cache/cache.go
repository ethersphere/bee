// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storer/internal"
	"github.com/ethersphere/bee/pkg/swarm"
	"resenje.org/multex"
)

var now = time.Now

// exported for migration
type CacheEntryItem = cacheEntry

const cacheEntrySize = swarm.HashSize + 8

var _ storage.Item = (*cacheEntry)(nil)

var (
	errMarshalCacheEntryInvalidAddress   = errors.New("marshal cacheEntry: invalid address")
	errMarshalCacheEntryInvalidTimestamp = errors.New("marshal cacheEntry: invalid timestamp")
	errUnmarshalCacheEntryInvalidSize    = errors.New("unmarshal cacheEntry: invalid size")
)

// Cache is the part of the localstore which keeps track of the chunks that are not
// part of the reserve but are potentially useful to store for obtaining bandwidth
// incentives.
type Cache struct {
	size      atomic.Int64
	capacity  int
	chunkLock *multex.Multex // protects storage ops at chunk level
	glock     sync.RWMutex   // blocks Get and Put ops while shallow copy is running.
}

// New creates a new Cache component with the specified capacity. The store is used
// here only to read the initial state of the cache before shutdown if there was
// any.
func New(ctx context.Context, store internal.Storage, capacity uint64) (*Cache, error) {
	count, err := store.IndexStore().Count(&cacheEntry{})
	if err != nil {
		return nil, fmt.Errorf("failed counting cache entries: %w", err)
	}

	c := &Cache{capacity: int(capacity)}
	c.size.Store(int64(count))
	c.chunkLock = multex.New()

	return c, nil
}

// Size returns the current size of the cache.
func (c *Cache) Size() uint64 {
	return uint64(c.size.Load())
}

// Capacity returns the capacity of the cache.
func (c *Cache) Capacity() uint64 { return uint64(c.capacity) }

// Putter returns a Storage.Putter instance which adds the chunk to the underlying
// chunkstore and also adds a Cache entry for the chunk.
func (c *Cache) Putter(store internal.Storage) storage.Putter {
	return storage.PutterFunc(func(ctx context.Context, chunk swarm.Chunk) error {

		c.chunkLock.Lock(chunk.Address().ByteString())
		defer c.chunkLock.Unlock(chunk.Address().ByteString())
		c.glock.RLock()
		defer c.glock.RUnlock()

		newEntry := &cacheEntry{Address: chunk.Address()}
		found, err := store.IndexStore().Has(newEntry)
		if err != nil {
			return fmt.Errorf("failed checking has cache entry: %w", err)
		}

		// if chunk is already part of cache, return found.
		if found {
			return nil
		}

		batch, err := store.IndexStore().Batch(ctx)
		if err != nil {
			return fmt.Errorf("failed creating batch: %w", err)
		}

		newEntry.AccessTimestamp = now().UnixNano()
		err = batch.Put(newEntry)
		if err != nil {
			return fmt.Errorf("failed adding cache entry: %w", err)
		}

		err = batch.Put(&cacheOrderIndex{
			Address:         newEntry.Address,
			AccessTimestamp: newEntry.AccessTimestamp,
		})
		if err != nil {
			return fmt.Errorf("failed adding cache order index: %w", err)
		}

		if err := batch.Commit(); err != nil {
			return fmt.Errorf("batch commit: %w", err)
		}

		err = store.ChunkStore().Put(ctx, chunk)
		if err != nil {
			return fmt.Errorf("failed adding chunk to chunkstore: %w", err)
		}

		c.size.Add(1)

		return nil
	})
}

// Getter returns a Storage.Getter instance which checks if the chunks accessed are
// part of cache it will update the cache indexes. If the operation to update the
// cache indexes fail, we need to fail the operation as this should signal the user
// of this getter to rollback the operation.
func (c *Cache) Getter(store internal.Storage) storage.Getter {
	return storage.GetterFunc(func(ctx context.Context, address swarm.Address) (swarm.Chunk, error) {

		ch, err := store.ChunkStore().Get(ctx, address)
		if err != nil {
			return nil, err
		}

		c.chunkLock.Lock(address.ByteString())
		defer c.chunkLock.Unlock(address.ByteString())
		c.glock.RLock()
		defer c.glock.RUnlock()

		// check if there is an entry in Cache. As this is the download path, we do
		// a best-effort operation. So in case of any error we return the chunk.
		entry := &cacheEntry{Address: address}
		err = store.IndexStore().Get(entry)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return ch, nil
			}
			return nil, fmt.Errorf("unexpected error getting indexstore entry: %w", err)
		}

		batch, err := store.IndexStore().Batch(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed creating batch: %w", err)
		}

		err = batch.Delete(&cacheOrderIndex{
			Address:         entry.Address,
			AccessTimestamp: entry.AccessTimestamp,
		})
		if err != nil {
			return nil, fmt.Errorf("failed deleting cache order index: %w", err)
		}

		entry.AccessTimestamp = now().UnixNano()
		err = batch.Put(&cacheOrderIndex{
			Address:         entry.Address,
			AccessTimestamp: entry.AccessTimestamp,
		})
		if err != nil {
			return nil, fmt.Errorf("failed adding cache order index: %w", err)
		}

		err = batch.Put(entry)
		if err != nil {
			return nil, fmt.Errorf("failed adding cache entry: %w", err)
		}

		err = batch.Commit()
		if err != nil {
			return nil, fmt.Errorf("batch commit: %w", err)
		}

		return ch, nil
	})
}

// ShallowCopy creates cache entries with the expectation that the chunk already exists in the chunkstore.
func (c *Cache) ShallowCopy(
	ctx context.Context,
	store internal.Storage,
	addrs ...swarm.Address,
) (err error) {

	c.glock.Lock()
	defer c.glock.Unlock()

	entries := make([]*cacheEntry, 0, len(addrs))

	defer func() {
		if err != nil {
			for _, entry := range entries {
				err = errors.Join(store.ChunkStore().Delete(context.Background(), entry.Address))
			}
		}
	}()

	for _, addr := range addrs {
		entry := &cacheEntry{Address: addr, AccessTimestamp: now().UnixNano()}
		if has, err := store.IndexStore().Has(entry); err == nil && has {
			// Since the caller has previously referenced the chunk (+1 refCnt), and if the chunk is already referenced
			// by the cache store (+1 refCnt), then we must decrement the refCnt by one ( -1 refCnt to bring the total to +1).
			// See https://github.com/ethersphere/bee/issues/4530.
			_ = store.ChunkStore().Delete(ctx, addr)
			continue
		}
		entries = append(entries, entry)
	}

	if len(entries) == 0 {
		return nil
	}

	//consider only the amount that can fit, the rest should be deleted from the chunkstore.
	if len(entries) > c.capacity {
		for _, addr := range entries[:len(entries)-c.capacity] {
			_ = store.ChunkStore().Delete(ctx, addr.Address)
		}
		entries = entries[len(entries)-c.capacity:]
	}

	batch, err := store.IndexStore().Batch(ctx)
	if err != nil {
		return fmt.Errorf("failed creating batch: %w", err)
	}

	for _, entry := range entries {
		err = batch.Put(entry)
		if err != nil {
			return fmt.Errorf("failed adding entry %s: %w", entry, err)
		}
		err = batch.Put(&cacheOrderIndex{
			Address:         entry.Address,
			AccessTimestamp: entry.AccessTimestamp,
		})
		if err != nil {
			return fmt.Errorf("failed adding cache order index: %w", err)
		}
	}

	if err := batch.Commit(); err != nil {
		return fmt.Errorf("batch commit: %w", err)
	}

	c.size.Add(int64(len(entries)))

	return nil
}

// Iterate iterates over entire cache with a call back.
func IterateCachedChunks(st storage.Store, callBackFunc func(address swarm.Address) error) error {
	return st.Iterate(storage.Query{
		Factory: func() storage.Item { return new(cacheEntry) },
	}, func(r storage.Result) (bool, error) {
		entry := r.Entry.(*cacheEntry)
		return false, callBackFunc(entry.Address)
	})
}

// RemoveOldest removes the oldest cache entries from the store. The count
// specifies the number of entries to remove.
func (c *Cache) RemoveOldest(ctx context.Context, store internal.Storage, chStore storage.ChunkStore, count uint64) error {
	return c.removeOldest(ctx, store, store.ChunkStore(), count, 1000)
}

func (c *Cache) removeOldest(ctx context.Context, store internal.Storage, chStore storage.ChunkStore, count uint64, batchCnt int) error {

	if count <= 0 {
		return nil
	}

	evictItems := make([]*cacheEntry, 0, count)
	err := store.IndexStore().Iterate(
		storage.Query{
			Factory:      func() storage.Item { return &cacheOrderIndex{} },
			ItemProperty: storage.QueryItemID,
		},
		func(res storage.Result) (bool, error) {
			accessTime, addr, err := idFromKey(res.ID)
			if err != nil {
				return false, fmt.Errorf("failed to parse cache order index %s: %w", res.ID, err)
			}
			entry := &cacheEntry{
				Address:         addr,
				AccessTimestamp: accessTime,
			}
			evictItems = append(evictItems, entry)
			count--
			return count == 0, nil
		},
	)
	if err != nil {
		return fmt.Errorf("failed iterating over cache order index: %w", err)
	}

	c.glock.Lock()
	defer c.glock.Unlock()

	for i := 0; i < len(evictItems); i += batchCnt {
		end := i + batchCnt
		if end > len(evictItems) {
			end = len(evictItems)
		}

		batch, err := store.IndexStore().Batch(ctx)
		if err != nil {
			return fmt.Errorf("failed creating batch: %w", err)
		}

		for _, entry := range evictItems[i:end] {
			err = batch.Delete(entry)
			if err != nil {
				return fmt.Errorf("failed deleting cache entry %s: %w", entry, err)
			}
			err = batch.Delete(&cacheOrderIndex{
				Address:         entry.Address,
				AccessTimestamp: entry.AccessTimestamp,
			})
			if err != nil {
				return fmt.Errorf("failed deleting cache order index %s: %w", entry.Address, err)
			}
			err = chStore.Delete(ctx, entry.Address)
			if err != nil {
				return fmt.Errorf("failed deleting chunk %s from chunkstore: %w", entry.Address, err)
			}
		}

		err = batch.Commit()
		if err != nil {
			return err
		}

		c.size.Add(-int64(end - i))
	}

	return nil
}

type cacheEntry struct {
	Address         swarm.Address
	AccessTimestamp int64
}

func (c *cacheEntry) ID() string { return c.Address.ByteString() }

func (cacheEntry) Namespace() string { return "cacheEntry" }

func (c *cacheEntry) Marshal() ([]byte, error) {
	entryBuf := make([]byte, cacheEntrySize)
	if c.Address.IsZero() {
		return nil, errMarshalCacheEntryInvalidAddress
	}
	if c.AccessTimestamp <= 0 {
		return nil, errMarshalCacheEntryInvalidTimestamp
	}
	copy(entryBuf[:swarm.HashSize], c.Address.Bytes())
	binary.LittleEndian.PutUint64(entryBuf[swarm.HashSize:], uint64(c.AccessTimestamp))
	return entryBuf, nil
}

func (c *cacheEntry) Unmarshal(buf []byte) error {
	if len(buf) != cacheEntrySize {
		return errUnmarshalCacheEntryInvalidSize
	}
	newEntry := new(cacheEntry)
	newEntry.Address = swarm.NewAddress(append(make([]byte, 0, swarm.HashSize), buf[:swarm.HashSize]...))
	newEntry.AccessTimestamp = int64(binary.LittleEndian.Uint64(buf[swarm.HashSize:]))
	*c = *newEntry
	return nil
}

func (c *cacheEntry) Clone() storage.Item {
	if c == nil {
		return nil
	}
	return &cacheEntry{
		Address:         c.Address.Clone(),
		AccessTimestamp: c.AccessTimestamp,
	}
}

func (c cacheEntry) String() string {
	return fmt.Sprintf(
		"cacheEntry { Address: %s AccessTimestamp: %s }",
		c.Address,
		time.Unix(c.AccessTimestamp, 0).UTC().Format(time.RFC3339),
	)
}

var _ storage.Item = (*cacheOrderIndex)(nil)

type cacheOrderIndex struct {
	AccessTimestamp int64
	Address         swarm.Address
}

func keyFromID(ts int64, addr swarm.Address) string {
	tsStr := fmt.Sprintf("%d", ts)
	return tsStr + addr.ByteString()
}

func idFromKey(key string) (int64, swarm.Address, error) {
	ts := key[:len(key)-swarm.HashSize]
	addr := key[len(key)-swarm.HashSize:]
	n, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return 0, swarm.ZeroAddress, err
	}
	return n, swarm.NewAddress([]byte(addr)), nil
}

func (c *cacheOrderIndex) ID() string {
	return keyFromID(c.AccessTimestamp, c.Address)
}

func (cacheOrderIndex) Namespace() string { return "cacheOrderIndex" }

func (cacheOrderIndex) Marshal() ([]byte, error) {
	return nil, nil
}

func (cacheOrderIndex) Unmarshal(_ []byte) error {
	return nil
}

func (c *cacheOrderIndex) Clone() storage.Item {
	if c == nil {
		return nil
	}
	return &cacheOrderIndex{
		AccessTimestamp: c.AccessTimestamp,
		Address:         c.Address.Clone(),
	}
}

func (c cacheOrderIndex) String() string {
	return fmt.Sprintf(
		"cacheOrderIndex { AccessTimestamp: %d Address: %s }",
		c.AccessTimestamp,
		c.Address.ByteString(),
	)
}
