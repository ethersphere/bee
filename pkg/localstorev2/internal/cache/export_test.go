package cache

import (
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
)

type (
	CacheEntry = cacheEntry
	CacheState = cacheState
)

var (
	ErrUnmarshalCacheStateInvalidSize  = errUnmarshalCacheStateInvalidSize
	ErrMarshalCacheEntryInvalidAddress = errMarshalCacheEntryInvalidAddress
	ErrUnmarshalCacheEntryInvalidSize  = errUnmarshalCacheEntryInvalidSize
)

func (c *Cache) State() (swarm.Address, swarm.Address, uint64) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	start := c.state.Start.Clone()
	end := c.state.End.Clone()

	return start, end, c.state.Count
}

func (c *Cache) IterateOldToNew(
	st storage.Store,
	start, end swarm.Address,
	iterateFn func(ch swarm.Address) (bool, error),
) error {

	currentAddr := start
	for !currentAddr.Equal(end) {
		entry := &cacheEntry{Address: currentAddr}
		err := st.Get(entry)
		if err != nil {
			return err
		}
		stop, err := iterateFn(entry.Address)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
		currentAddr = entry.Next
	}

	return nil
}
