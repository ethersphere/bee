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
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/feeds"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// DefaultLevels is the number of concurrent lookaheads
// 8 spans 2^8 updates
const DefaultLevels = 8

var (
	_ feeds.Index   = (*index)(nil)
	_ feeds.Lookup  = (*finder)(nil)
	_ feeds.Lookup  = (*asyncFinder)(nil)
	_ feeds.Updater = (*updater)(nil)
)

// index just wraps a uint64. implements the feeds.Index interface
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

// Next requires
func (i *index) Next(last int64, at uint64) feeds.Index {
	return &index{i.index + 1}
}

// finder encapsulates a chunk store getter and a feed and provides
// non-concurrent lookup
type finder struct {
	getter *feeds.Getter
}

// NewFinder constructs an finder (feeds.Lookup interface)
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
		// if timestamp is later than the `at` target datetime, then return previous chunk  and index
		if ts > uint64(at) {
			return ch, &index{i - 1}, &index{i}, nil
		}
		ch = u
	}
}

// asyncFinder encapsulates a chunk store getter and a feed and provides
//  non-concurrent lookup
type asyncFinder struct {
	getter *feeds.Getter
}

// NewAsyncFinder constructs an AsyncFinder
func NewAsyncFinder(getter storage.Getter, feed *feeds.Feed) feeds.Lookup {
	return &asyncFinder{feeds.NewGetter(getter, feed)}
}

// interval represents a batch of concurrent retreieve requests
// that probe the interval (base,b+2^level) at offsets 2^k-1 for k=1,...,max
// recording  the level of the latest found update chunk and the earliest not found update
// the actual latest update is guessed to be within a subinterval
type interval struct {
	base     uint64  // beginning of the interval, guaranteed to have an  update
	level    int     // maximum level to check
	found    *result // the result with the latest chunk found
	notFound int     // the earliest level where no update is found
}

// when a subinterval is identified to contain the latest update
// next returns an interval matching it
func (i *interval) next() *interval {
	found := i.found.level
	i.found.level = 0
	return &interval{
		base:     i.found.index, // set base to index of latest chunk found
		level:    found,         // set max level to the latest update level
		notFound: found,         // set notFound to the latest update level
		found:    i.found,       // inherit latest found  result
	}
}

func (i *interval) retry() *interval {
	r := i.next()
	r.level = i.level    // reset to max
	r.notFound = i.level //  reset to max
	return r
}

func newInterval(base uint64) *interval {
	return &interval{base: base, level: DefaultLevels, notFound: DefaultLevels}
}

// results capture a chunk lookup on a interval
type result struct {
	chunk    swarm.Chunk // the chunk found
	interval *interval   // the interval it belongs to
	level    int         // the level within the interval
	index    uint64      // the actual sequence index of the update
}

// At looks up the version valid at time `at`
// after is a unix time hint of the latest known update
func (f *asyncFinder) At(ctx context.Context, at, after int64) (ch swarm.Chunk, cur, next feeds.Index, err error) {
	// first lookup update at the 0 index
	// TODO: consider receive after as uint
	ch, err = f.get(ctx, at, uint64(after))
	if err != nil {
		return nil, nil, nil, err
	}
	if ch == nil {
		return nil, nil, &index{uint64(after)}, nil
	}
	// if chunk exists construct an initial interval with base=0
	c := make(chan *result)
	i := newInterval(0)
	i.found = &result{ch, nil, 0, 0}

	quit := make(chan struct{})
	defer close(quit)

	// launch concurrent request at  doubling intervals
	go f.at(ctx, at, 0, i, c, quit)
	for r := range c {
		// collect the results into the interval
		i = r.interval
		if r.chunk == nil {
			if i.notFound < r.level {
				continue
			}
			i.notFound = r.level - 1
		} else {
			if i.found.level > r.level {
				continue
			}
			// if a chunk is found on the max level, and this is already a subinterval
			// then found.index+1 is already known to be not found
			if i.level == r.level && r.level < DefaultLevels {
				return r.chunk, &index{r.index}, &index{r.index + 1}, nil
			}
			i.found = r
		}
		// below applies even if i.latest==ceilingLevel in which case we just continue with
		// DefaultLevel lookaheads
		if i.found.level == i.notFound {
			if i.found.level == 0 {
				return i.found.chunk, &index{i.found.index}, &index{i.found.index + 1}, nil
			}
			go f.at(ctx, at, 0, i.next(), c, quit)
		}
		// inconsistent feed, retry
		if i.notFound < i.found.level {
			go f.at(ctx, at, i.found.level, i.retry(), c, quit)
		}
	}
	return nil, nil, nil, nil
}

// at launches concurrent lookups at exponential intervals after the starting from further
func (f *asyncFinder) at(ctx context.Context, at int64, min int, i *interval, c chan<- *result, quit <-chan struct{}) {
	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(i.level)
	for l := i.level; l > min; l-- {
		select {
		case <-quit: // if the parent process quit
			return
		default:
		}
		go func(l int) {
			// TODO: remove hardcoded timeout and define it as constant or inject in the getter.
			reqCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
			defer func() {
				wg.Done()
				cancel()
			}()
			index := i.base + (1 << l) - 1
			chunk := f.asyncGet(reqCtx, at, index)

			select {
			case ch := <-chunk:
				c <- &result{ch, i, l, index}
			case <-reqCtx.Done():
				c <- &result{nil, i, l, index}
			case <-quit:
			}
		}(l)
	}
}

func (f *asyncFinder) asyncGet(ctx context.Context, at int64, index uint64) <-chan swarm.Chunk {
	c := make(chan swarm.Chunk)
	go func() {
		defer close(c)
		ch, err := f.get(ctx, at, index)
		if err != nil {
			return
		}
		c <- ch
	}()
	return c
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
