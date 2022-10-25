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

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
	"resenje.org/singleflight"
)

type Cache struct {
	sd    p2p.StreamerDisconnecter
	sf    singleflight.Group
	cache sync.Map // "${overlay}/swarm/pushsync/1.1.0/pushsync" -> *CachedStream{}
}

func New(sd p2p.StreamerDisconnecter) *Cache {
	return &Cache{
		sd: sd,
	}
}

func (s *Cache) NetworkStatus() p2p.NetworkStatus {
	return s.sd.NetworkStatus()
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

	// full close every stream
	for _, key := range keys {
		_, _, _ = s.sf.Do(context.Background(), key, func(ctx context.Context) (res interface{}, err error) {
			mapStream, found := s.cache.LoadAndDelete(key)
			if !found {
				return
			}

			stream := mapStream.(*CachedStream)

			_ = stream.stream.FullClose() // TODO bubble up the error?

			return
		})
	}

	return s.sd.Disconnect(overlay, reason)
}

func (s *Cache) Blocklist(overlay swarm.Address, duration time.Duration, reason string) error {
	// TODO update cache here?
	return s.sd.Blocklist(overlay, duration, reason)
}

func (s *Cache) NewStream(ctx context.Context, address swarm.Address, h p2p.Headers, protocol, version, stream string) (p2p.Stream, error) {
	streamFullName := p2p.NewSwarmStreamName(protocol, version, stream)

	sfKey := address.String() + streamFullName

	res, _, err := s.sf.Do(ctx, sfKey, func(ctx context.Context) (interface{}, error) {
		cachedStream, found := s.cache.Load(sfKey)
		if !found {
			newStream, err := s.sd.NewStream(ctx, address, h, protocol, version, stream)
			if err != nil {
				return nil, err
			}

			cachedStream = &CachedStream{stream: newStream}
			s.cache.Store(sfKey, cachedStream)

			return cachedStream, nil
		}

		return cachedStream.(*CachedStream), nil
	})

	if err != nil {
		return nil, err
	}

	return res.(p2p.Stream), nil
}
