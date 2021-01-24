package epochs

import (
	"context"
	"errors"

	"github.com/ethersphere/bee/pkg/feeds"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Finder encapsulates a chunk store getter and a feed and provides
//  non-concurrent lookup methods
type Finder struct {
	getter *feeds.Getter
}

// NewFinder constructs an AsyncFinder
func NewFinder(getter storage.Getter, feed *feeds.Feed) feeds.Lookup {
	return &Finder{feeds.NewGetter(getter, feed)}
}

// At looks up the version valid at time `at`
// after is a unix time hint of the latest known update
func (f *Finder) At(ctx context.Context, at, after int64) (swarm.Chunk, error) {
	e, ch, err := f.common(ctx, at, after)
	if err != nil {
		return nil, err
	}
	return f.at(ctx, uint64(at), e, ch)
}

// common returns the lowest common ancestor for which a feed update chunk is found in the chunk store
func (f *Finder) common(ctx context.Context, at, after int64) (*epoch, swarm.Chunk, error) {
	for e := lca(at, after); ; e = e.parent() {
		ch, err := f.getter.Get(ctx, e)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				if e.level == maxLevel {
					return e, nil, nil
				}
				continue
			}
			return e, nil, err
		}
		ts, err := feeds.UpdatedAt(ch)
		if err != nil {
			return e, nil, err
		}
		if ts <= uint64(at) {
			return e, ch, nil
		}
	}
}

// at is a non-concurrent recursive Finder function to find the version update chunk at time `at`
func (f *Finder) at(ctx context.Context, at uint64, e *epoch, ch swarm.Chunk) (swarm.Chunk, error) {
	uch, err := f.getter.Get(ctx, e)
	if err != nil {
		// error retrieving
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
		// epoch not found on branch
		if e.isLeft() { // no lower resolution
			return ch, nil
		}
		// traverse earlier branch
		return f.at(ctx, e.start-1, e.left(), ch)
	}
	// epoch found
	// check if timestamp is later then target
	ts, err := feeds.UpdatedAt(uch)
	if err != nil {
		return nil, err
	}
	if ts > at {
		if e.isLeft() {
			return ch, nil
		}
		return f.at(ctx, e.start-1, e.left(), ch)
	}
	if e.level == 0 { // matching update time or finest resolution
		return uch, nil
	}
	// continue traversing based on at
	return f.at(ctx, at, e.childAt(at), uch)
}

type result struct {
	path  *path
	chunk swarm.Chunk
	*epoch
}

// AsyncFinder encapsulates a chunk store getter and a feed and provides
//  non-concurrent lookup methods
type AsyncFinder struct {
	getter *feeds.Getter
}

type path struct {
	at     int64
	top    *result
	bottom *result
	cancel chan struct{}
}

func newPath(at int64) *path {
	return &path{at, nil, nil, make(chan struct{})}
}

// NewAsyncFinder constructs an AsyncFinder
func NewAsyncFinder(getter storage.Getter, feed *feeds.Feed) feeds.Lookup {
	return &AsyncFinder{feeds.NewGetter(getter, feed)}
}

// at attempts to retrieve all epoch chunks on the path for `at` concurrently
func (f *AsyncFinder) at(ctx context.Context, at int64, p *path, e *epoch, c chan *result) {
	for ; ; e = e.childAt(uint64(at)) {
		select {
		case <-p.cancel:
			return
		default:
		}
		go func(e *epoch) {
			uch, _ := f.getter.Get(ctx, e)
			c <- &result{p, uch, e}
		}(e)
		if e.level == 0 {
			return
		}
	}
}

// At looks up the version valid at time `at`
// after is a unix time hint of the latest known update
func (f *AsyncFinder) At(ctx context.Context, at, after int64) (swarm.Chunk, error) {
	c := make(chan *result)
	go f.at(ctx, at, newPath(at), &epoch{0, maxLevel}, c)
LOOP:
	for r := range c {
		p := r.path
		// ignore result from paths already  cancelled
		select {
		case <-p.cancel:
			continue LOOP
		default:
		}
		if r.chunk != nil { // update chunk for epoch found
			if r.level == 0 { // return if deepest level epoch
				return r.chunk, nil
			}
			// ignore if higher level than the deepest epoch found
			if p.top != nil && p.top.level < r.level {
				continue LOOP
			}
			// check if timestamp is later than target time
			ts, err := feeds.UpdatedAt(r.chunk)
			if err != nil {
				return nil, err
			}
			if ts <= uint64(p.at) { // valid for latest update before `at`
				p.top = r
			} else if p.bottom == nil || p.bottom.level < r.level {
				p.bottom = r
			}
		} else { // update chunk for epoch not found
			// if top level than return with no update found
			if r.level == 32 {
				return nil, nil
			}
			// if topmost epoch not found, then set bottom
			if p.bottom == nil || p.bottom.level < r.level {
				p.bottom = r
			}
		}

		// found - not found for two consecutive epochs
		if p.top != nil && p.bottom != nil && p.top.level == p.bottom.level+1 {
			// cancel path
			close(p.cancel)
			if p.bottom.isLeft() {
				return p.top.chunk, nil
			}
			// recursive call on new path through left sister
			np := newPath(at)
			np.top = &result{np, p.top.chunk, p.top.epoch}
			go f.at(ctx, int64(p.bottom.start-1), np, p.bottom.left(), c)
		}
	}
	return nil, nil
}
