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
	size     atomic.Int64
	capacity int

	chunkLock  *multex.Multex // protects storage ops at chunk level
	removeLock sync.RWMutex   // blocks Get and Put ops while cache items are being evicted.
}

// New creates a new Cache component with the specified capacity. The store is used
// here only to read the initial state of the cache before shutdown if there was
// any.
func New(ctx context.Context, store storage.Reader, capacity uint64) (*Cache, error) {
	count, err := store.Count(&cacheEntry{})
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
		c.removeLock.RLock()
		defer c.removeLock.RUnlock()

		trx, done := store.NewTransaction()
		defer done()

		newEntry := &cacheEntry{Address: chunk.Address()}
		found, err := trx.IndexStore().Has(newEntry)
		if err != nil {
			return fmt.Errorf("failed checking has cache entry: %w", err)
		}

		// if chunk is already part of cache, return found.
		if found {
			return nil
		}

		newEntry.AccessTimestamp = now().UnixNano()
		err = trx.IndexStore().Put(newEntry)
		if err != nil {
			return fmt.Errorf("failed adding cache entry: %w", err)
		}

		err = trx.IndexStore().Put(&cacheOrderIndex{
			Address:         newEntry.Address,
			AccessTimestamp: newEntry.AccessTimestamp,
		})
		if err != nil {
			return fmt.Errorf("failed adding cache order index: %w", err)
		}

		err = trx.ChunkStore().Put(ctx, chunk)
		if err != nil {
			return fmt.Errorf("failed adding chunk to chunkstore: %w", err)
		}

		if err := trx.Commit(); err != nil {
			return fmt.Errorf("batch commit: %w", err)
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

		c.chunkLock.Lock(address.ByteString())
		defer c.chunkLock.Unlock(address.ByteString())
		c.removeLock.RLock()
		defer c.removeLock.RUnlock()

		trx, done := store.NewTransaction()
		defer done()

		ch, err := trx.ChunkStore().Get(ctx, address)
		if err != nil {
			return nil, err
		}

		// check if there is an entry in Cache. As this is the download path, we do
		// a best-effort operation. So in case of any error we return the chunk.
		entry := &cacheEntry{Address: address}
		err = trx.IndexStore().Get(entry)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return ch, nil
			}
			return nil, fmt.Errorf("unexpected error getting indexstore entry: %w", err)
		}

		err = trx.IndexStore().Delete(&cacheOrderIndex{
			Address:         entry.Address,
			AccessTimestamp: entry.AccessTimestamp,
		})
		if err != nil {
			return nil, fmt.Errorf("failed deleting cache order index: %w", err)
		}

		entry.AccessTimestamp = now().UnixNano()
		err = trx.IndexStore().Put(&cacheOrderIndex{
			Address:         entry.Address,
			AccessTimestamp: entry.AccessTimestamp,
		})
		if err != nil {
			return nil, fmt.Errorf("failed adding cache order index: %w", err)
		}

		err = trx.IndexStore().Put(entry)
		if err != nil {
			return nil, fmt.Errorf("failed adding cache entry: %w", err)
		}

		err = trx.Commit()
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

	c.removeLock.Lock()
	defer c.removeLock.Unlock()

	defer func() {
		if err != nil {
			err = store.Run(func(s internal.Store) error {
				var rerr error
				for _, addr := range addrs {
					rerr = errors.Join(rerr, s.ChunkStore().Delete(context.Background(), addr))
				}
				return rerr
			})
		}
	}()

	//consider only the amount that can fit, the rest should be deleted from the chunkstore.
	if len(addrs) > c.capacity {

		_ = store.Run(func(s internal.Store) error {
			for _, addr := range addrs[:len(addrs)-c.capacity] {
				_ = s.ChunkStore().Delete(ctx, addr)
			}
			return nil
		})
		addrs = addrs[len(addrs)-c.capacity:]
	}

	entriesToAdd := make([]*cacheEntry, 0, len(addrs))

	err = store.Run(func(s internal.Store) error {
		for _, addr := range addrs {
			entry := &cacheEntry{Address: addr, AccessTimestamp: now().UnixNano()}
			if has, err := s.IndexStore().Has(entry); err == nil && has {
				continue
			}
			entriesToAdd = append(entriesToAdd, entry)
		}

		for _, entry := range entriesToAdd {
			err = s.IndexStore().Put(entry)
			if err != nil {
				return fmt.Errorf("failed adding entry %s: %w", entry, err)
			}
			err = s.IndexStore().Put(&cacheOrderIndex{
				Address:         entry.Address,
				AccessTimestamp: entry.AccessTimestamp,
			})
			if err != nil {
				return fmt.Errorf("failed adding cache order index: %w", err)
			}
		}

		return nil
	})
	if err == nil {
		c.size.Add(int64(len(entriesToAdd)))
	}

	return err
}

// RemoveOldest removes the oldest cache entries from the store. The count
// specifies the number of entries to remove.
func (c *Cache) RemoveOldest(
	ctx context.Context,
	st internal.Storage,
	count uint64,
) error {
	if count <= 0 {
		return nil
	}

	c.removeLock.Lock()
	defer c.removeLock.Unlock()

	evictItems := make([]*cacheEntry, 0, count)
	err := st.ReadOnly().IndexStore().Iterate(
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

	batchCnt := 1_000

	for i := 0; i < len(evictItems); i += batchCnt {
		end := i + batchCnt
		if end > len(evictItems) {
			end = len(evictItems)
		}

		err := st.Run(func(s internal.Store) error {
			for _, entry := range evictItems[i:end] {
				err = s.IndexStore().Delete(entry)
				if err != nil {
					return fmt.Errorf("failed deleting cache entry %s: %w", entry, err)
				}
				err = s.IndexStore().Delete(&cacheOrderIndex{
					Address:         entry.Address,
					AccessTimestamp: entry.AccessTimestamp,
				})
				if err != nil {
					return fmt.Errorf("failed deleting cache order index %s: %w", entry.Address, err)
				}
				err = s.ChunkStore().Delete(ctx, entry.Address)
				if err != nil {
					return fmt.Errorf("failed deleting chunk %s from chunkstore: %w", entry.Address, err)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}

		c.size.Add(-int64(len(evictItems)))
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
