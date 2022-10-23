package cache

import (
	"context"
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

type cacheStart struct {
	Address swarm.Address
}

func (cacheStart) Namespace() string { return "start" }

func (cacheStart) ID() string { return "e" }

func (c *cacheStart) Marshal() ([]byte, error) { return nil, nil }

func (c *cacheStart) Unmarshal(buf []byte) error { return nil }

type cacheEnd struct {
	Address swarm.Address
}

func (cacheEnd) Namespace() string { return "end" }

func (cacheEnd) ID() string { return "e" }

func (c *cacheEnd) Marshal() ([]byte, error) { return nil, nil }

func (c *cacheEnd) Unmarshal(buf []byte) error { return nil }

type cacheSize struct {
	Size uint64
}

func (cacheSize) Namespace() string { return "size" }

func (cacheSize) ID() string { return "e" }

func (c *cacheSize) Marshal() ([]byte, error) { return nil, nil }

func (c *cacheSize) Unmarshal(buf []byte) error { return nil }

type cache struct {
	mtx      sync.Mutex
	capacity uint64
}

func (c *cache) popFront(
	ctx context.Context,
	store storage.Store,
	chStore storage.ChunkStore,
) error {
	start := &cacheStart{}
	err := store.Get(start)
	if err != nil {
		return err
	}
	expiredEntry := &cacheEntry{Address: start.Address}
	err = store.Get(expiredEntry)
	if err != nil {
		return err
	}
	start.Address = expiredEntry.Next
	err = store.Put(start)
	if err != nil {
		return err
	}
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
	end := &cacheEnd{}
	err := store.Get(end)
	if err != nil {
		return err
	}

	entry := &cacheEntry{Address: end.Address}
	err = store.Get(entry)
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

	end.Address = newEntry.Address
	err = store.Put(end)
	if err != nil {
		return err
	}

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
		// if chunk is already part of cache, return found
		found, err := store.Has(newEntry)
		if err != nil {
			return false, err
		}

		if found {
			return true, nil
		}

		err = c.pushBack(ctx, newEntry, chunk, store, chStore)
		if err != nil {
			return false, err
		}

		size := &cacheSize{}
		err = store.Get(size)
		if err != nil {
			return false, err
		}

		if size.Size == c.capacity {
			err = c.popFront(ctx, store, chStore)
		} else {
			size.Size++
			err = store.Put(size)
		}
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

			prev := &cacheEntry{Address: entry.Prev}
			next := &cacheEntry{Address: entry.Next}

			err = store.Get(prev)
			if err != nil {
				return err
			}

			err = store.Get(next)
			if err != nil {
				return err
			}

			err = c.pushBack(ctx, entry, nil, store, chStore)
			if err != nil {
				return err
			}

			next.Prev = prev.Address
			prev.Next = next.Address

			err = store.Put(prev)
			if err != nil {
				return err
			}

			err = store.Put(next)
			if err != nil {
				return err
			}

			return nil
		}()
		if err != nil {
			// log error
		}

		return ch, nil
	})
}
