// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pushsync provides the pushsync protocol
// implementation.
package streamcache

import (
	"context"
	"strings"
	"sync"
	"time"

	"container/list"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
)

type Cache struct {
	sd    p2p.StreamerDisconnecter
	mu    sync.Mutex
	cache sync.Map // "${overlay}/swarm/pushsync/1.1.0/pushsync" -> queue of *CachedStream{}
}

type cacheItem struct {
	ss *list.List
	mu sync.Mutex
}

func New(sd p2p.StreamerDisconnecter) *Cache {
	list.New()
	return &Cache{
		sd: sd,
	}
}

func (s *Cache) Disconnect(overlay swarm.Address, reason string) error {
	addressKey := overlay.String()
	var keys []string

	// collect the keys for the overlay
	s.cache.Range(func(key, _ interface{}) bool {
		k := key.(string)
		if strings.HasPrefix(k, addressKey) {
			keys = append(keys, k)
		}
		return true
	})

	for _, key := range keys {
		if cs, found := s.cache.LoadAndDelete(key); found {
			streams := cs.(*cacheItem)
			f := streams.ss.Front()
			for {
				if f == nil {
					break
				}
				_ = f.Value.(*CachedStream).stream.FullClose()
				f = f.Next()
			}
		}
	}

	return s.sd.Disconnect(overlay, reason)
}

func (s *Cache) NewStream(ctx context.Context, address swarm.Address, h p2p.Headers, protocol, version, stream string) (p2p.Stream, error) {
	streamFullName := p2p.NewSwarmStreamName(protocol, version, stream)

	sfKey := address.String() + streamFullName

	ci, found := s.cache.Load(sfKey)
	if !found {
		// vast majority of cases the item will be found
		// so it makes sense to minimize
		s.mu.Lock()
		// check if no other thread has put to cache while
		// we were waiting for the lock
		ci, found = s.cache.Load(sfKey)

		if !found {
			ci = &cacheItem{
				ss: list.New(),
			}
			s.cache.Store(sfKey, ci)
		}

		s.mu.Unlock()
	}

	item := ci.(*cacheItem)

	item.mu.Lock()
	defer item.mu.Unlock()

	strm := item.ss.Front()

	if strm == nil {
		newStream, err := s.sd.NewStream(ctx, address, h, protocol, version, stream)
		if err != nil {
			return nil, err
		}

		cs := &CachedStream{stream: newStream}

		// callback for putting the stream back in cache
		cs.ret = func() {
			item.mu.Lock()
			defer item.mu.Unlock()
			item.ss.PushBack(cs)
		}

		return cs, nil
	}

	item.ss.Remove(strm)

	return strm.Value.(p2p.Stream), nil
}

func (s *Cache) NetworkStatus() p2p.NetworkStatus {
	return s.sd.NetworkStatus()
}

func (s *Cache) Blocklist(overlay swarm.Address, duration time.Duration, reason string) error {
	return s.sd.Blocklist(overlay, duration, reason)
}
