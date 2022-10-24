package cache

import (
	"context"
	"fmt"
	"sync"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
)

type cacheEntry struct {
	Address swarm.Address
	Prev    swarm.Address
	Next    swarm.Address
}

func (cacheEntry) Namespace() string { return "entry" }

func (c *cacheEntry) ID() string { return c.Address.ByteString() }

func (c *cacheEntry) Marshal() ([]byte, error) { return nil, nil }

func (c *cacheEntry) Unmarshal(buf []byte) error { return nil }

type cacheState struct {
	Start swarm.Address
	End   swarm.Address
	Count uint64
}

func (cacheState) Namespace() string { return "state" }

func (cacheState) ID() string { return "e" }

func (c *cacheState) Marshal() ([]byte, error) { return nil, nil }

func (c *cacheState) Unmarshal(buf []byte) error { return nil }

type cache struct {
	mtx      sync.Mutex
	state    *cacheState
	capacity uint64
}

func New(store storage.Store, capacity uint64) (*cache, error) {
	state := &cacheState{}
	err := store.Get(state)
	if err != nil {
		return nil, fmt.Errorf("failed reading cache state: %w", err)
	}

	if capacity < state.Count {
	}

	return &cache{state: state, capacity: capacity}, nil
}

func (c *cache) popFront(
	ctx context.Context,
	store storage.Store,
	chStore storage.ChunkStore,
) error {
	expiredEntry := &cacheEntry{Address: c.state.Start}
	err := store.Get(expiredEntry)
	if err != nil {
		return err
	}

	c.state.Start = expiredEntry.Next

	err = store.Delete(expiredEntry)
	if err != nil {
		return err
	}
	err = chStore.Delete(ctx, expiredEntry.Address)
	if err != nil {
		return err
	}

	return nil
}

func (c *cache) pushBack(
	ctx context.Context,
	newEntry *cacheEntry,
	chunk swarm.Chunk,
	store storage.Store,
	chStore storage.ChunkStore,
) error {
	entry := &cacheEntry{Address: c.state.End}
	err := store.Get(entry)
	if err != nil {
		return err
	}

	entry.Next = newEntry.Address
	newEntry.Prev = entry.Address

	err = store.Put(newEntry)
	if err != nil {
		return err
	}

	err = store.Put(entry)
	if err != nil {
		return err
	}

	c.state.End = newEntry.Address

	_, err = chStore.Put(ctx, chunk)
	if err != nil {
		return err
	}

	return nil
}

func (c *cache) Putter(store storage.Store, chStore storage.ChunkStore) storage.Putter {
	return storage.PutterFunc(func(ctx context.Context, chunk swarm.Chunk) (bool, error) {
		c.mtx.Lock()
		defer c.mtx.Unlock()

		newEntry := &cacheEntry{Address: chunk.Address()}
		found, err := store.Has(newEntry)
		if err != nil {
			return false, err
		}

		// if chunk is already part of cache, return found
		if found {
			return true, nil
		}

		err = c.pushBack(ctx, newEntry, chunk, store, chStore)
		if err != nil {
			return false, err
		}

		if c.state.Count == c.capacity {
			err = c.popFront(ctx, store, chStore)
			if err != nil {
				return false, err
			}
		} else {
			c.state.Count++
		}
		err = store.Put(c.state)
		if err != nil {
			return false, err
		}

		return false, nil
	})
}

func (c *cache) Getter(store storage.Store, chStore storage.ChunkStore) storage.Getter {
	return storage.GetterFunc(func(ctx context.Context, address swarm.Address) (swarm.Chunk, error) {
		ch, err := chStore.Get(ctx, address)
		if err != nil {
			return nil, err
		}

		entry := &cacheEntry{Address: address}
		err = store.Get(entry)
		if err != nil {
			return ch, nil
		}

		err = func() error {
			c.mtx.Lock()
			defer c.mtx.Unlock()

			if c.state.End.Equal(address) {
				return nil
			}

			prev := &cacheEntry{Address: entry.Prev}
			next := &cacheEntry{Address: entry.Next}

			if !c.state.Start.Equal(address) {
				err = store.Get(prev)
				if err != nil {
					return err
				}
			}

			err = store.Get(next)
			if err != nil {
				return err
			}

			err = c.pushBack(ctx, entry, nil, store, chStore)
			if err != nil {
				return err
			}

			if !c.state.Start.Equal(address) {
				prev.Next = next.Address
				err = store.Put(prev)
				if err != nil {
					return err
				}
			}

			next.Prev = prev.Address
			err = store.Put(next)
			if err != nil {
				return err
			}

			if c.state.Start.Equal(address) {
				c.state.Start = next.Address
			}

			return store.Put(c.state)
		}()
		if err != nil {
			// log error
		}

		return ch, nil
	})
}
