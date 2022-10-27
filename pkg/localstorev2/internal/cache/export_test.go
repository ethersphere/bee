package cache

import "github.com/ethersphere/bee/pkg/swarm"

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

	start := swarm.NewAddress(append(make([]byte, 0, swarm.HashSize), c.state.Start.Bytes()...))
	end := swarm.NewAddress(append(make([]byte, 0, swarm.HashSize), c.state.End.Bytes()...))

	return start, end, c.state.Count
}
