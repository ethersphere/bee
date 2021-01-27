package sequence

import (
	"context"
	"encoding/binary"
	"errors"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/feeds"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type index struct {
	index uint64
}

func (i *index) MarshalBinary() ([]byte, error) {
	indexBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(indexBytes, i.index)
	return indexBytes, nil
}

// Finder encapsulates a chunk store getter and a feed and provides
//  non-concurrent lookup methods
type Finder struct {
	getter *feeds.Getter
}

// NewFinder constructs an Finder
func NewFinder(getter storage.Getter, feed *feeds.Feed) feeds.Lookup {
	return &Finder{feeds.NewGetter(getter, feed)}
}

// At looks up the version valid at time `at`
// after is a unix time hint of the latest known update
func (f *Finder) At(ctx context.Context, at, after int64) (ch swarm.Chunk, err error) {
	for i := uint64(0); ; i++ {
		u, err := f.getter.Get(ctx, &index{i})
		if err != nil {
			if !errors.Is(err, storage.ErrNotFound) {
				return nil, err
			}
			return ch, nil
		}
		ts, err := feeds.UpdatedAt(u)
		if err != nil {
			return nil, err
		}
		if ts > uint64(at) {
			return ch, nil
		}
		ch = u
	}
}

// Finder encapsulates a chunk store getter and a feed and provides
//  non-concurrent lookup methods
type AsyncFinder struct {
	getter *feeds.Getter
}

// NewAsyncFinder constructs an AsyncFinder
func NewAsyncFinder(getter storage.Getter, feed *feeds.Feed) feeds.Lookup {
	return &AsyncFinder{feeds.NewGetter(getter, feed)}
}

type path struct {
	latest result
	base   uint64
	level  int
	cancel chan struct{}
}

func newPath(base uint64) *path {
	return &path{base: base, cancel: make(chan struct{})}
}

type result struct {
	chunk swarm.Chunk
	path  *path
	level int
	seq   uint64
	diff  int64
}

// At looks up the version valid at time `at`
// after is a unix time hint of the latest known update
func (f *AsyncFinder) At(ctx context.Context, at, after int64) (ch swarm.Chunk, err error) {
	ch, diff, err := f.get(ctx, at, 0)
	if err != nil {
		return nil, err
	}
	if ch == nil {
		return nil, nil
	}
	if diff == 0 {
		return ch, nil
	}
	c := make(chan result)
	p := newPath(0)
	p.latest.chunk = ch
	for p.level = 1; diff>>p.level > 0; p.level++ {
	}
	quit := make(chan struct{})
	defer close(quit)
	go f.at(ctx, at, p, c, quit)
	for r := range c {
		p = r.path
		if r.chunk == nil {
			if r.level == 0 {
				return p.latest.chunk, nil
			}
			if p.level < r.level {
				continue
			}
			p.level = r.level - 1
		} else {
			if p.cancel != nil {
				close(p.cancel)
				p.cancel = nil
			}
			if r.diff == 0 {
				return r.chunk, nil
			}
			if p.latest.level > r.level {
				continue
			}
			p.latest = r
		}
		// below applies even  if  p.latest==maxLevel
		if p.latest.level == p.level {
			if p.level == 0 {
				if p.cancel != nil {
					close(p.cancel)
					p.cancel = nil
				}
				return p.latest.chunk, nil
			}
			np := newPath(p.latest.seq)
			np.level = p.level
			np.latest.chunk = p.latest.chunk
			go f.at(ctx, at, np, c, quit)
		}
	}
	return nil, nil
}

func (f *AsyncFinder) at(ctx context.Context, at int64, p *path, c chan result, quit chan struct{}) {
	for i := p.level; i > 0; i-- {
		select {
		case <-p.cancel:
			return
		case <-quit:
			return
		default:
		}
		go func(i int) {
			seq := p.base + (1 << i) - 1
			ch, diff, err := f.get(ctx, at, seq)
			if err != nil {
				return
			}
			select {
			case c <- result{ch, p, i, seq, diff}:
			case <-quit:
			}
		}(i)
	}
}

func (f *AsyncFinder) get(ctx context.Context, at int64, seq uint64) (swarm.Chunk, int64, error) {
	u, err := f.getter.Get(ctx, &index{seq})
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, 0, err
		}
		return nil, 0, nil
	}
	ts, err := feeds.UpdatedAt(u)
	if err != nil {
		return nil, 0, err
	}
	diff := at - int64(ts)
	if diff < 0 {
		return nil, 0, nil
	}
	return u, diff, nil
}

// Updater encapsulates a feeds putter to generate successive updates for epoch based feeds
// it persists the last update
type Updater struct {
	*feeds.Putter
	next uint64
}

// NewUpdater constructs a feed updater
func NewUpdater(putter storage.Putter, signer crypto.Signer, topic string) (feeds.Updater, error) {
	p, err := feeds.NewPutter(putter, signer, topic)
	if err != nil {
		return nil, err
	}
	return &Updater{Putter: p}, nil
}

// Update pushes an update to the feed through the chunk stores
func (u *Updater) Update(ctx context.Context, at int64, payload []byte) error {
	err := u.Put(ctx, &index{u.next}, at, payload)
	if err != nil {
		return err
	}
	u.next++
	return nil
}

func (u *Updater) Feed() *feeds.Feed {
	return u.Putter.Feed
}
