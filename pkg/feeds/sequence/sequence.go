package sequence

import (
	"context"
	"encoding/binary"
	"errors"
	"time"

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
	for i := uint64(1); ; i++ {
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
	ch, diff, err := f.get(ctx, at, 1)
	if err != nil {
		return nil, err
	}
	if ch == nil {
		return nil, nil
	}
	last := uint64(0)
	c := make(chan result)
	p := newPath(last)
	p.latest.chunk = ch
	for p.level = 1; diff>>p.level > 0; p.level++ {
	}
	go f.at(ctx, at, p, c)
	for r := range c {
		if r.chunk == nil {
			if r.level == 0 {
				return r.path.latest.chunk, nil
			}
			if r.path.level < r.level {
				continue
			}
			r.path.level = r.level - 1
		} else {
			if r.path.cancel != nil {
				close(r.path.cancel)
				r.path.cancel = nil
			}
			if r.diff == 0 {
				return r.chunk, nil
			}
			if r.path.latest.level <= r.level {
				r.path.latest = r
			}
		}
		// below applies even  if  r.path.latest==maxLevel
		if r.path.latest.level == r.path.level {
			if r.path.level == 0 {
				return r.path.latest.chunk, nil
			}
			np := newPath(r.path.latest.seq - 1)
			np.level = r.path.level
			np.latest.chunk = r.path.latest.chunk
			go f.at(ctx, at, np, c)
		}
	}
	return nil, nil
}

func (f *AsyncFinder) at(ctx context.Context, at int64, p *path, c chan result) {
	for i := p.level; i > 0; i-- {
		select {
		case <-p.cancel:
			return
		default:
		}
		go func(i int) {
			seq := p.base + 1<<i
			ch, diff, err := f.get(ctx, at, seq)
			if err != nil {
				return
			}
			select {
			case c <- result{ch, p, i, seq, diff}:
			case <-time.After(time.Minute):
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
	last uint64
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
	err := u.Put(ctx, &index{u.last + 1}, at, payload)
	if err != nil {
		return err
	}
	u.last++
	return nil
}

func (u *Updater) Feed() *feeds.Feed {
	return u.Putter.Feed
}
