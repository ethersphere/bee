// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sequence provides implementation of sequential indexing for
// feeds
// this feed type is best suited for
// - version updates
// - followed updates
// - frequent or regular-interval updates
package sequence

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/feeds"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
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

// NewIndex returns the feeds.Index of the sequential update at position i.
func NewIndex(i uint64) feeds.Index {
	return &index{i}
}

func (i *index) String() string {
	return strconv.FormatUint(i.index, 10)
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

// At looks for incremental feed updates from 0 upwards, if it does not find one, it assumes that the last found update is the most recent
func (f *finder) At(ctx context.Context, at int64, _ uint64) (ch swarm.Chunk, current, next feeds.Index, err error) {
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
		ch = u
	}
}

// asyncFinder encapsulates a chunk store getter and a feed and provides
// non-concurrent lookup
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

// At looks up the latest update of the feed.
// after is the index of a known update to start the search from; if there is
// no update at that index, the search starts from index 0.
func (f *asyncFinder) At(ctx context.Context, at int64, after uint64) (ch swarm.Chunk, cur, next feeds.Index, err error) {
	// first lookup update at the start index
	ch, err = f.get(ctx, at, after)
	if err != nil {
		return nil, nil, nil, err
	}
	if ch == nil {
		if after > 0 {
			// the start index hint does not point to an update, search from the beginning
			return f.At(ctx, at, 0)
		}
		return nil, nil, &index{0}, nil
	}
	// if chunk exists construct an initial interval with base=after
	c := make(chan *result)
	i := newInterval(after)
	i.found = &result{ch, nil, 0, after}

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
				return r.chunk, &index{r.index}, nextIndex(r.index), nil
			}
			i.found = r
		}
		// below applies even if i.latest==ceilingLevel in which case we just continue with
		// DefaultLevel lookaheads
		if i.found.level == i.notFound {
			if i.found.level == 0 {
				return i.found.chunk, &index{i.found.index}, nextIndex(i.found.index), nil
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

// nextIndex returns the index after i, or nil when i is the last possible index.
func nextIndex(i uint64) feeds.Index {
	if i == math.MaxUint64 {
		return nil
	}
	return &index{i + 1}
}

// at launches concurrent lookups at exponential intervals after the starting from further
func (f *asyncFinder) at(ctx context.Context, at int64, minValue int, i *interval, c chan<- *result, quit <-chan struct{}) {
	var wg sync.WaitGroup

	for l := i.level; l > minValue; l-- {
		select {
		case <-quit: // if the parent process quit
			return
		default:
		}

		wg.Add(1)
		go func(l int) {
			// TODO: remove hardcoded timeout and define it as constant or inject in the getter.
			reqCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
			defer func() {
				cancel()
				wg.Done()
			}()
			offset := uint64(1)<<l - 1
			if offset > math.MaxUint64-i.base {
				// the index would overflow, there can be no update beyond the end of the index space
				select {
				case c <- &result{nil, i, l, 0}:
				case <-quit:
				}
				return
			}
			index := i.base + offset
			chunk := f.asyncGet(reqCtx, at, index)

			select {
			case ch := <-chunk:
				select {
				case c <- &result{ch, i, l, index}:
				case <-quit:
					return
				}
			case <-reqCtx.Done():
				select {
				case c <- &result{nil, i, l, index}:
				case <-quit:
					return
				}
			case <-quit:
			}
		}(l)
	}

	wg.Wait()
}

func (f *asyncFinder) asyncGet(ctx context.Context, at int64, index uint64) <-chan swarm.Chunk {
	c := make(chan swarm.Chunk, 1)
	go func() {
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
	return u, nil
}

// updater encapsulates a feeds putter to generate successive updates for sequence based feeds
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
	err := u.Put(ctx, &index{u.next}, payload)
	if err != nil {
		return err
	}
	u.next++
	return nil
}

func (u *updater) Feed() *feeds.Feed {
	return u.Putter.Feed
}
