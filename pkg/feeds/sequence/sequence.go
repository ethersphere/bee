// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sequence provides implementation of sequential indexing for
// time-based feeds
// this feed type is best suited for
// - version updates
// - followed updates
// - frequent or regular-interval updates
package sequence

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/feeds"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const DefaultLevels = 8

var _ feeds.Index = (*index)(nil)
var _ feeds.Lookup = (*finder)(nil)
var _ feeds.Lookup = (*asyncFinder)(nil)
var _ feeds.Updater = (*updater)(nil)

type index struct {
	index uint64
}

func (i *index) String() string {
	return fmt.Sprintf("%d", i.index)
}

func (i *index) MarshalBinary() ([]byte, error) {
	indexBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(indexBytes, i.index)
	return indexBytes, nil
}

func (i *index) Next(last int64, at uint64) feeds.Index {
	return &index{i.index + 1}
}

// finder encapsulates a chunk store getter and a feed and provides
//  non-concurrent lookup methods
type finder struct {
	getter *feeds.Getter
}

// NewFinder constructs an Finder
func NewFinder(getter storage.Getter, feed *feeds.Feed) feeds.Lookup {
	return &finder{feeds.NewGetter(getter, feed)}
}

// At looks up the version valid at time `at`
// after is a unix time hint of the latest known update
func (f *finder) At(ctx context.Context, at, after int64) (ch swarm.Chunk, current, next feeds.Index, err error) {
	for i := uint64(0); ; i++ {
		u, err := f.getter.Get(ctx, &index{i})
		if err != nil {
			if !errors.Is(err, storage.ErrNotFound) {
				return nil, nil, nil, err
			}
			if i > 0 {
				current = &index{i - 1}
			}
			return ch, current, &index{i}, nil
		}
		ts, err := feeds.UpdatedAt(u)
		if err != nil {
			return nil, nil, nil, err
		}
		if ts > uint64(at) {
			return ch, &index{i - 1}, nil, nil
		}
		ch = u
	}
}

// asyncFinder encapsulates a chunk store getter and a feed and provides
//  non-concurrent lookup methods
type asyncFinder struct {
	getter *feeds.Getter
}

// NewAsyncFinder constructs an AsyncFinder
func NewAsyncFinder(getter storage.Getter, feed *feeds.Feed) feeds.Lookup {
	return &asyncFinder{feeds.NewGetter(getter, feed)}
}

// path represents a series of exponential lookahead lookups
// - starting from base + 1 as level 1
// - upto base + 2^DefaultLevel - 1
type path struct {
	chunk     swarm.Chunk // the chunk found
	index     uint64
	base      uint64
	level     int
	min       int
	max       int
	cancel    chan struct{}
	cancelled bool
}

// path can be closed if there is a returned chunk
func (p *path) close() {
	if !p.cancelled {
		close(p.cancel)
		p.cancelled = true
	}
}
func (p *path) next() *path {
	return &path{
		base:   p.index,
		index:  p.index,
		max:    p.max,
		level:  p.level,
		chunk:  p.chunk,
		cancel: make(chan struct{}),
	}
}
func newPath(base uint64) *path {
	return &path{base: base, cancel: make(chan struct{}), level: DefaultLevels, max: DefaultLevels}
}

// results capture a chunk lookup on a path
type result struct {
	chunk swarm.Chunk // the chunk found
	path  *path       // for a request
	level int         // the
	index uint64      // the actual seqeuence in seq
}

// At looks up the version valid at time `at`
// after is a unix time hint of the latest known update
func (f *asyncFinder) At(ctx context.Context, at, after int64) (ch swarm.Chunk, cur, next feeds.Index, err error) {
	ch, err = f.get(ctx, at, 0)
	if err != nil {
		return nil, nil, nil, err
	}
	if ch == nil {
		return nil, nil, &index{0}, nil
	}
	c := make(chan result)
	pa := newPath(0)
	pa.chunk = ch
	quit := make(chan struct{})
	defer close(quit)
	go f.at(ctx, at, pa, c, quit)
	for r := range c {
		// r.path ~ tagged which path it comes from
		// collect the results into the path
		p := r.path
		if r.chunk == nil {
			if p.max < r.level {
				continue
			}
			p.max = r.level - 1
		} else {
			p.close()
			if p.min > r.level { // ignore lower than
				continue
			}
			// if there is a chunk for this path, then the  latest chunk has surely beed sent
			// since `at` starts from log interval
			if p.level == r.level && r.level < DefaultLevels {
				return r.chunk, &index{r.index}, &index{r.index + 1}, nil
			}
			p.min = r.level
			p.chunk = r.chunk
			p.index = r.index
		}
		// below applies even  if  p.latest==maxLevel in which case we just continue with
		// DefaultLevel lookaheads
		if p.min == p.max {
			if p.min == 0 {
				return p.chunk, &index{p.index}, &index{p.index + 1}, nil
			}
			np := p.next()
			go f.at(ctx, at, np, c, quit)
		}
		if p.max < p.min {
			np := p.next()
			np.level = p.level
			np.max = p.level
			go f.at(ctx, at, np, c, quit)
		}
	}
	return nil, nil, nil, nil
}

// at launches concurrent lookups at exponential intervals after th c starting from further
func (f *asyncFinder) at(ctx context.Context, at int64, p *path, c chan<- result, quit <-chan struct{}) {
	for i := p.level; i > 0; i-- {
		select {
		case <-p.cancel:
			return
		case <-quit:
			return
		default:
		}
		go func(i int) {
			index := p.base + (1 << i) - 1
			ch, err := f.get(ctx, at, index)
			if err != nil {
				return
			}
			select {
			case c <- result{ch, p, i, index}:
			case <-quit:
			}
		}(i)
	}
}

// get performs a lookup of an update chunk, returns nil (not error) if not found
func (f *asyncFinder) get(ctx context.Context, at int64, idx uint64) (swarm.Chunk, error) {
	u, err := f.getter.Get(ctx, &index{idx})
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
		// if 'not-found' error, then just silence and return nil chunk
		return nil, nil
	}
	ts, err := feeds.UpdatedAt(u)
	if err != nil {
		return nil, err
	}
	// this means the update timestamp is later than the pivot time we are looking for
	// handled as if the update was missing but with no uncertainty due to timeout
	if at < int64(ts) {
		return nil, nil
	}
	return u, nil
}

// updater encapsulates a feeds putter to generate successive updates for epoch based feeds
// it persists the last update
type updater struct {
	*feeds.Putter
	next uint64
}

// NewUpdater constructs a feed updater
func NewUpdater(putter storage.Putter, signer crypto.Signer, topic []byte) (feeds.Updater, error) {
	p, err := feeds.NewPutter(putter, signer, topic)
	if err != nil {
		return nil, err
	}
	return &updater{Putter: p}, nil
}

// Update pushes an update to the feed through the chunk stores
func (u *updater) Update(ctx context.Context, at int64, payload []byte) error {
	err := u.Put(ctx, &index{u.next}, at, payload)
	if err != nil {
		return err
	}
	u.next++
	return nil
}

func (u *updater) Feed() *feeds.Feed {
	return u.Putter.Feed
}
