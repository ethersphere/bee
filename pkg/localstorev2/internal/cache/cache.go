package cache

import (
	"bytes"
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

var (
	errMarshalCacheEntryInvalidAddress = errors.New("marshal: cacheEntry invalid address")
	errUnmarshalCacheEntryInvalidSize  = errors.New("unmarshal: cacheEntry invalid size")
	errUnmarshalCacheStateInvalidSize  = errors.New("unmarshal: cacheState invalid size")
)

func getAddressOrZero(buf []byte) swarm.Address {
	emptyAddr := make([]byte, swarm.HashSize)
	if bytes.Equal(buf, emptyAddr) {
		return swarm.ZeroAddress
	}
	return swarm.NewAddress(append(make([]byte, 0, swarm.HashSize), buf...))
}

func addressBytesOrZero(addr swarm.Address) []byte {
	if addr.IsZero() {
		return make([]byte, swarm.HashSize)
	}
	return addr.Bytes()
}

type cacheEntry struct {
	Address swarm.Address
	Prev    swarm.Address
	Next    swarm.Address
}

func (cacheEntry) Namespace() string { return "entry" }

func (c *cacheEntry) ID() string { return c.Address.ByteString() }

func (c *cacheEntry) Marshal() ([]byte, error) {
	entryBuf := make([]byte, cacheEntrySize)
	if c.Address.IsZero() {
		return nil, errMarshalCacheEntryInvalidAddress
	}
	copy(entryBuf[:swarm.HashSize], c.Address.Bytes())
	copy(entryBuf[swarm.HashSize:2*swarm.HashSize], addressBytesOrZero(c.Prev))
	copy(entryBuf[2*swarm.HashSize:], addressBytesOrZero(c.Next))
	return entryBuf, nil
}

func (c *cacheEntry) Unmarshal(buf []byte) error {
	if len(buf) != cacheEntrySize {
		return errUnmarshalCacheEntryInvalidSize
	}
	newEntry := new(cacheEntry)
	newEntry.Address = swarm.NewAddress(append(make([]byte, 0, swarm.HashSize), buf[:swarm.HashSize]...))
	newEntry.Prev = getAddressOrZero(buf[swarm.HashSize : 2*swarm.HashSize])
	newEntry.Next = getAddressOrZero(buf[2*swarm.HashSize:])
	*c = *newEntry

	return nil
}

const cacheStateSize = 2*swarm.HashSize + 8

type cacheState struct {
	Start swarm.Address
	End   swarm.Address
	Count uint64
}

func (cacheState) Namespace() string { return "state" }

func (cacheState) ID() string { return "e" }

func (c *cacheState) Marshal() ([]byte, error) {
	entryBuf := make([]byte, cacheStateSize)
	copy(entryBuf[:swarm.HashSize], addressBytesOrZero(c.Start))
	copy(entryBuf[swarm.HashSize:2*swarm.HashSize], addressBytesOrZero(c.End))
	binary.LittleEndian.PutUint64(entryBuf[2*swarm.HashSize:], c.Count)
	return entryBuf, nil
}

func (c *cacheState) Unmarshal(buf []byte) error {
	if len(buf) != cacheStateSize {
		return errUnmarshalCacheStateInvalidSize
	}
	newEntry := new(cacheState)
	newEntry.Start = getAddressOrZero(buf[:swarm.HashSize])
	newEntry.End = getAddressOrZero(buf[swarm.HashSize : 2*swarm.HashSize])
	newEntry.Count = binary.LittleEndian.Uint64(buf[2*swarm.HashSize:])
	*c = *newEntry
	return nil
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
func New(storg internal.Storage, capacity uint64) (*Cache, error) {
	state := &cacheState{}
	err := storg.Store().Get(state)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("failed reading cache state: %w", err)
	}

	if capacity < state.Count {
		entry := &cacheEntry{Address: state.Start}
		var i uint64
		for i = 0; i < (state.Count - capacity); i++ {
			err = storg.Store().Get(entry)
			if err != nil {
				return nil, fmt.Errorf("failed reading cache entry %s: %w", state.Start, err)
			}
			err = storg.ChunkStore().Delete(storg.Ctx(), entry.Address)
			if err != nil {
				return nil, fmt.Errorf("failed deleting chunk %s: %w", entry.Address, err)
			}
			err = storg.Store().Delete(entry)
			if err != nil {
				return nil, fmt.Errorf("failed deleting cache entry item: %w", err)
			}
			entry.Address = entry.Next
			state.Start = entry.Next.Clone()
			state.Count--
		}
		err = storg.Store().Put(state)
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
	expiredEntry := &cacheEntry{Address: c.state.Start}
	err := store.Get(expiredEntry)
	if err != nil {
		return err
	}

	// remove the chunk.
	err = chStore.Delete(ctx, expiredEntry.Address)
	if err != nil {
		return err
	}

	// delete the item.
	err = store.Delete(expiredEntry)
	if err != nil {
		return err
	}

	// update new front.
	c.state.Start = expiredEntry.Next.Clone()

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
	// read the last entry.
	entry := &cacheEntry{Address: c.state.End}
	err := store.Get(entry)
	if err != nil {
		return err
	}

	// update the pointers.
	entry.Next = newEntry.Address.Clone()
	newEntry.Prev = entry.Address.Clone()

	// add the new chunk to chunkstore if requested.
	if chunk != nil {
		_, err = chStore.Put(ctx, chunk)
		if err != nil {
			return err
		}
	}

	// add the new item.
	err = store.Put(newEntry)
	if err != nil {
		return err
	}

	// update the old last entry.
	err = store.Put(entry)
	if err != nil {
		return err
	}

	// update state.
	c.state.End = newEntry.Address.Clone()

	return nil
}

// Putter returns a Storage.Putter instance which adds the chunk to the underlying
// chunkstore and also adds a Cache entry for the chunk.
func (c *Cache) Putter(store storage.Store, chStore storage.ChunkStore) storage.Putter {
	return storage.PutterFunc(func(ctx context.Context, chunk swarm.Chunk) (bool, error) {
		c.mtx.Lock()
		defer c.mtx.Unlock()

		newEntry := &cacheEntry{Address: chunk.Address()}
		found, err := store.Has(newEntry)
		if err != nil {
			return false, err
		}

		// if chunk is already part of cache, return found.
		if found {
			return true, nil
		}

		if c.state.Count != 0 {
			// if we are here, this is a new chunk which should be added. Add it to
			// the end.
			err = c.pushBack(ctx, newEntry, chunk, store, chStore)
			if err != nil {
				return false, err
			}
		} else {
			_, err = chStore.Put(ctx, chunk)
			if err != nil {
				return false, err
			}
			err = store.Put(newEntry)
			if err != nil {
				return false, err
			}
			c.state.Start = newEntry.Address.Clone()
			c.state.End = newEntry.Address.Clone()
		}

		if c.state.Count == c.capacity {
			// if we reach the full capacity, remove the first element.
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

// Getter returns a Storage.Getter instance which checks if the chunks accessed are
// part of cache it will update the cache queue. If the operation to update the
// cache indexes fail, we need to fail the operation as this should signal the user
// of this getter to rollback the operation.
func (c *Cache) Getter(store storage.Store, chStore storage.ChunkStore) storage.Getter {
	return storage.GetterFunc(func(ctx context.Context, address swarm.Address) (swarm.Chunk, error) {
		ch, err := chStore.Get(ctx, address)
		if err != nil {
			return nil, err
		}

		// check if there is an entry in Cache. As this is the download path, we do
		// a best-effort operation. So in case of any error we return the chunk.
		entry := &cacheEntry{Address: address}
		err = store.Get(entry)
		if err != nil {
			return ch, nil
		}

		oldState := *c.state

		err = func() error {
			c.mtx.Lock()
			defer c.mtx.Unlock()

			// if the chunk is already the last return early.
			if c.state.End.Equal(address) {
				return nil
			}

			prev := &cacheEntry{Address: entry.Prev}
			next := &cacheEntry{Address: entry.Next}

			// move the chunk to the end due to the access. Dont send duplicate
			// chunk put operation.
			err = c.pushBack(ctx, entry, nil, store, chStore)
			if err != nil {
				return err
			}

			// if this is first chunk we dont need to update both the prev or the
			// next.
			if !c.state.Start.Equal(address) {
				err = store.Get(prev)
				if err != nil {
					return err
				}
				err = store.Get(next)
				if err != nil {
					return err
				}
				prev.Next = next.Address.Clone()
				err = store.Put(prev)
				if err != nil {
					return err
				}

				next.Prev = prev.Address.Clone()
				err = store.Put(next)
				if err != nil {
					return err
				}
			}

			if c.state.Start.Equal(address) {
				c.state.Start = next.Address.Clone()
			}

			return store.Put(c.state)
		}()
		if err != nil {
			// we need to return error here as this operation is generally guarded
			// by some sort of trasaction. These updates need to be reverted.
			*c.state = oldState
			return nil, fmt.Errorf("failed updating cache indexes: %w", err)
		}

		return ch, nil
	})
}
