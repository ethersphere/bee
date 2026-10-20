// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// hintKey identifies a range of a peer in the hint cache.
type hintKey struct {
	peer string
	r    fingerprint.RangeKey
}

// hintCache keeps the size of the last decoded difference per (peer, range)
// for the symbol estimate hint(R) (proposal §6.3), recorded by the decoding
// and by the serving side of a stream: in memory only, the least
// recently used of HintCacheSize entries evicted. Losing an entry only makes
// a first batch smaller. It is safe for concurrent use.
type hintCache struct {
	cache *lru.Cache[hintKey, uint64]
}

func newHintCache(size int) *hintCache {
	c, err := lru.New[hintKey, uint64](size)
	if err != nil {
		panic(err) // only for size <= 0
	}
	return &hintCache{cache: c}
}

// peer returns the Hints of peer.
func (h *hintCache) peer(peer swarm.Address) Hints {
	return peerHints{c: h.cache, peer: peer.ByteString()}
}

type peerHints struct {
	c    *lru.Cache[hintKey, uint64]
	peer string
}

func (p peerHints) Hint(r fingerprint.Range) uint64 {
	d, _ := p.c.Get(hintKey{peer: p.peer, r: r.Key()})
	return d
}

func (p peerHints) SetHint(r fingerprint.Range, d uint64) {
	p.c.Add(hintKey{peer: p.peer, r: r.Key()}, d)
}
