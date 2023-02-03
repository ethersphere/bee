// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/localstorev2/internal"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
)

const cacheEntrySize = 3 * swarm.HashSize

var _ storage.Item = (*cacheEntry)(nil)

var (
	errMarshalCacheEntryInvalidAddress = errors.New("marshal cacheEntry: invalid address")
	errUnmarshalCacheEntryInvalidSize  = errors.New("unmarshal cacheEntry: invalid size")
	errUnmarshalCacheStateInvalidSize  = errors.New("unmarshal cacheState: invalid size")
)

type cacheEntry struct {
	Address swarm.Address
	Prev    swarm.Address
	Next    swarm.Address
}

func (c *cacheEntry) ID() string { return c.Address.ByteString() }

func (cacheEntry) Namespace() string { return "cacheEntry" }

func (c *cacheEntry) Marshal() ([]byte, error) {
	entryBuf := make([]byte, cacheEntrySize)
	if c.Address.IsZero() {
		return nil, errMarshalCacheEntryInvalidAddress
	}
	copy(entryBuf[:swarm.HashSize], c.Address.Bytes())
	copy(entryBuf[swarm.HashSize:2*swarm.HashSize], internal.AddressBytesOrZero(c.Prev))
	copy(entryBuf[2*swarm.HashSize:], internal.AddressBytesOrZero(c.Next))
	return entryBuf, nil
}

func (c *cacheEntry) Unmarshal(buf []byte) error {
	if len(buf) != cacheEntrySize {
		return errUnmarshalCacheEntryInvalidSize
	}
	newEntry := new(cacheEntry)
	newEntry.Address = swarm.NewAddress(append(make([]byte, 0, swarm.HashSize), buf[:swarm.HashSize]...))
	newEntry.Prev = internal.AddressOrZero(buf[swarm.HashSize : 2*swarm.HashSize])
	newEntry.Next = internal.AddressOrZero(buf[2*swarm.HashSize:])
	*c = *newEntry
	return nil
}

func (c *cacheEntry) Clone() storage.Item {
	if c == nil {
		return nil
	}
	return &cacheEntry{
		Address: c.Address.Clone(),
		Prev:    c.Prev.Clone(),
		Next:    c.Next.Clone(),
	}
}

func (c cacheEntry) String() string {
	return fmt.Sprintf("cacheEntry { Address: %s Prev: %s Next: %s }", c.Address, c.Prev, c.Next)
}

const cacheStateSize = 2*swarm.HashSize + 8

var _ storage.Item = (*cacheState)(nil)

type cacheState struct {
	Head  swarm.Address
	Tail  swarm.Address
	Count uint64
}

func (cacheState) ID() string { return "entry" }

func (cacheState) Namespace() string { return "cacheState" }

func (c *cacheState) Marshal() ([]byte, error) {
	entryBuf := make([]byte, cacheStateSize)
	copy(entryBuf[:swarm.HashSize], internal.AddressBytesOrZero(c.Head))
	copy(entryBuf[swarm.HashSize:2*swarm.HashSize], internal.AddressBytesOrZero(c.Tail))
	binary.LittleEndian.PutUint64(entryBuf[2*swarm.HashSize:], c.Count)
	return entryBuf, nil
}

func (c *cacheState) Unmarshal(buf []byte) error {
	if len(buf) != cacheStateSize {
		return errUnmarshalCacheStateInvalidSize
	}
	newEntry := new(cacheState)
	newEntry.Head = internal.AddressOrZero(buf[:swarm.HashSize])
	newEntry.Tail = internal.AddressOrZero(buf[swarm.HashSize : 2*swarm.HashSize])
	newEntry.Count = binary.LittleEndian.Uint64(buf[2*swarm.HashSize:])
	*c = *newEntry
	return nil
}

func (c *cacheState) Clone() storage.Item {
	if c == nil {
		return nil
	}
	return &cacheState{
		Head:  c.Head.Clone(),
		Tail:  c.Tail.Clone(),
		Count: c.Count,
	}
}

func (c cacheState) String() string {
	return fmt.Sprintf("cacheState { Head: %s Tail: %s Count: %d }", c.Head, c.Tail, c.Count)
}

// Cache is the part of the localstore which keeps track of the chunks that are not
// part of the reserve but are potentially useful to store for obtaining bandwidth
// incentives. In order to avoid GC we will only keep track of a fixed no. of chunks
// as part of the cache and evict a chunk as soon as we go above capacity. In order
// to achieve this we will use some additional cache state in-mem and stored on disk
// to create a double-ended queue. The typical operations required here would be
// a pushBack which adds an item to the end and popFront, which removed the first item.
// The different operations:
// 1. New item will be added to the end.
// 2. Item pushed to end on access.
// 3. Removal happens from the front.
type Cache struct {
	mtx      sync.Mutex
	state    *cacheState
	capacity uint64
}

// New creates a new Cache component with the specified capacity. The store is used
// here only to read the initial state of the cache before shutdown if there was
// any.
func New(ctx context.Context, store internal.Storage, capacity uint64) (*Cache, error) {
	state := &cacheState{}
	err := store.IndexStore().Get(state)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("failed reading cache state: %w", err)
	}

	if capacity < state.Count {
		entry := &cacheEntry{Address: state.Head}
		var i uint64
		itemsToRemove := state.Count - capacity
		for i = 0; i < itemsToRemove; i++ {
			err = store.IndexStore().Get(entry)
			if err != nil {
				return nil, fmt.Errorf("failed reading cache entry %s: %w", state.Head, err)
			}
			err = store.ChunkStore().Delete(ctx, entry.Address)
			if err != nil {
				return nil, fmt.Errorf("failed deleting chunk %s: %w", entry.Address, err)
			}
			err = store.IndexStore().Delete(entry)
			if err != nil {
				return nil, fmt.Errorf("failed deleting cache entry item %s: %w", entry, err)
			}
			entry.Address = entry.Next
			state.Head = entry.Next.Clone()
			state.Count--
		}
		err = store.IndexStore().Put(state)
		if err != nil {
			return nil, fmt.Errorf("failed updating state: %w", err)
		}
	}

	return &Cache{state: state, capacity: capacity}, nil
}

// popFront will pop the first item in the queue. It will update the cache state so
// should be called under lock. The state will not be updated to DB here as there
// could be more changes involved, so the caller has to take care of updating the
// cache state.
func (c *Cache) popFront(
	ctx context.Context,
	store storage.Store,
	chStore storage.ChunkStore,
) error {
	// read the first entry.
	expiredEntry := &cacheEntry{Address: c.state.Head}
	err := store.Get(expiredEntry)
	if err != nil {
		return fmt.Errorf("failed getting old head entry %s: %w", c.state.Head, err)
	}

	// remove the chunk.
	err = chStore.Delete(ctx, expiredEntry.Address)
	if err != nil {
		return fmt.Errorf("failed deleting chunk %s from chunkstore: %w", expiredEntry.Address, err)
	}

	// delete the item.
	err = store.Delete(expiredEntry)
	if err != nil {
		return fmt.Errorf("failed deleting old head entry %s: %w", expiredEntry, err)
	}

	// update new front.
	c.state.Head = expiredEntry.Next.Clone()

	return nil
}

// pushBack will add the new entry to the end of the queue. It will update the state
// so again should be called under lock. Also, we could pushBack an entry which is
// already existing, in which case a duplicate chunk Put operation need not be done.
// The state will not be updated to DB here as there could be more potential changes.
func (c *Cache) pushBack(
	ctx context.Context,
	newEntry *cacheEntry,
	chunk swarm.Chunk,
	store storage.Store,
	chStore storage.ChunkStore,
) error {
	// read the tail entry.
	entry := &cacheEntry{Address: c.state.Tail}
	err := store.Get(entry)
	if err != nil {
		return fmt.Errorf("failed reading tail entry %s: %w", c.state.Tail, err)
	}

	// update the pointers.
	entry.Next = newEntry.Address.Clone()
	newEntry.Prev = entry.Address.Clone()

	// add the new chunk to chunkstore if requested.
	if chunk != nil {
		err = chStore.Put(ctx, chunk)
		if err != nil {
			return fmt.Errorf("failed to add chunk to chunkstore %s: %w", chunk.Address(), err)
		}
	}

	// add the new item.
	err = store.Put(newEntry)
	if err != nil {
		return fmt.Errorf("failed adding the new cacheEntry %s: %w", newEntry, err)
	}

	// update the old tail entry.
	err = store.Put(entry)
	if err != nil {
		return fmt.Errorf("failed updating the old tail entry %s: %w", entry, err)
	}

	// update state.
	c.state.Tail = newEntry.Address.Clone()

	return nil
}

// Putter returns a Storage.Putter instance which adds the chunk to the underlying
// chunkstore and also adds a Cache entry for the chunk.
func (c *Cache) Putter(store internal.Storage) storage.Putter {
	return storage.PutterFunc(func(ctx context.Context, chunk swarm.Chunk) error {
		newEntry := &cacheEntry{Address: chunk.Address()}
		found, err := store.IndexStore().Has(newEntry)
		if err != nil {
			return fmt.Errorf("failed checking has cache entry: %w", err)
		}

		// if chunk is already part of cache, return found.
		if found {
			return nil
		}

		c.mtx.Lock()
		defer c.mtx.Unlock()

		// save the old state. All the operations here are expected to be guarded by
		// some store transaction, so on error return, all updates are rollbacked. As
		// a result we will revert to the old state if there is any error. Any changes
		// to the state will create newly allocated entries, so a shallow copy is enough.
		oldState := *c.state

		err = func() error {
			if c.state.Count != 0 {
				// if we are here, this is a new chunk which should be added. Add it to
				// the end.
				err = c.pushBack(ctx, newEntry, chunk, store.IndexStore(), store.ChunkStore())
				if err != nil {
					return err
				}
			} else {
				// first entry
				err = store.ChunkStore().Put(ctx, chunk)
				if err != nil {
					return fmt.Errorf("failed adding chunk %s to chunkstore: %w", chunk.Address(), err)
				}
				err = store.IndexStore().Put(newEntry)
				if err != nil {
					return fmt.Errorf("failed adding new cacheEntry %s: %w", newEntry, err)
				}
				c.state.Head = newEntry.Address.Clone()
				c.state.Tail = newEntry.Address.Clone()
			}

			if c.state.Count == c.capacity {
				// if we reach the full capacity, remove the first element.
				err = c.popFront(ctx, store.IndexStore(), store.ChunkStore())
				if err != nil {
					return err
				}
			} else {
				c.state.Count++
			}
			err = store.IndexStore().Put(c.state)
			if err != nil {
				return fmt.Errorf("failed updating state %s: %w", c.state, err)
			}
			return nil
		}()
		if err != nil {
			*c.state = oldState
			return fmt.Errorf("cache put: %w", err)
		}

		return nil
	})
}

// Getter returns a Storage.Getter instance which checks if the chunks accessed are
// part of cache it will update the cache queue. If the operation to update the
// cache indexes fail, we need to fail the operation as this should signal the user
// of this getter to rollback the operation.
func (c *Cache) Getter(store internal.Storage) storage.Getter {
	return storage.GetterFunc(func(ctx context.Context, address swarm.Address) (swarm.Chunk, error) {
		ch, err := store.ChunkStore().Get(ctx, address)
		if err != nil {
			return nil, err
		}

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

		c.mtx.Lock()
		defer c.mtx.Unlock()

		oldState := *c.state

		err = func() error {

			// if the chunk is already the tail return early.
			if c.state.Tail.Equal(address) {
				return nil
			}

			prev := &cacheEntry{Address: entry.Prev}
			next := &cacheEntry{Address: entry.Next}

			// move the chunk to the end due to the access. Dont send duplicate
			// chunk put operation.
			err := c.pushBack(ctx, entry, nil, store.IndexStore(), store.ChunkStore())
			if err != nil {
				return err
			}

			// if this is first chunk we dont need to update both the prev or the
			// next.
			if !c.state.Head.Equal(address) {
				err = store.IndexStore().Get(prev)
				if err != nil {
					return fmt.Errorf("failed getting previous entry %s: %w", prev, err)
				}
				err = store.IndexStore().Get(next)
				if err != nil {
					return fmt.Errorf("failed getting next entry %s: %w", next, err)
				}
				prev.Next = next.Address.Clone()
				err = store.IndexStore().Put(prev)
				if err != nil {
					return fmt.Errorf("failed updating prev entry %s: %w", prev, err)
				}

				next.Prev = prev.Address.Clone()
				err = store.IndexStore().Put(next)
				if err != nil {
					return fmt.Errorf("failed updating next entry %s: %w", next, err)
				}
			}

			if c.state.Head.Equal(address) {
				c.state.Head = next.Address.Clone()
			}

			err = store.IndexStore().Put(c.state)
			if err != nil {
				return fmt.Errorf("failed updating state %s: %w", c.state, err)
			}
			return nil
		}()
		if err != nil {
			*c.state = oldState
			return nil, fmt.Errorf("cache get: %w", err)
		}

		return ch, nil
	})
}
