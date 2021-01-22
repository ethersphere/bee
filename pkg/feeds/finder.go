package feeds

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Finder encapsulates a chunk store getter and a feed
type Finder struct {
	storage.Getter
	*Feed
}

// NewFinder constructs a feed finderg
func NewFinder(getter storage.Getter, feed *Feed) *Finder {
	return &Finder{getter, feed}
}

// Latest looks up the latest update of the feed
// after is a unix time hint of the latest known update
func (f *Finder) Latest(ctx context.Context, after int64) (swarm.Chunk, error) {
	return f.At(ctx, time.Now().Unix(), after)
}

// At looks up the version valid at time `at`
// after is a unix time hint of the latest known update
func (f *Finder) At(ctx context.Context, at, after int64) (swarm.Chunk, error) {
	// fmt.Printf("find common %d %d\n", at, after)
	e, ch, err := f.common(ctx, at, after)
	if err != nil {
		return nil, err
	}
	// fmt.Printf("at %d\n", at)
	return f.at(ctx, uint64(at), e, ch)
}

// get creates an update of the underlying feed at the given epoch
// and looks it up in the chunk store based on its address
func (l *Finder) get(ctx context.Context, e *epoch) (swarm.Chunk, error) {
	u := &update{l.Feed, e}
	addr, err := u.address()
	if err != nil {
		return nil, err
	}
	// fmt.Println("find chunk addr ", addr.String())
	return l.Get(ctx, storage.ModeGetRequest, addr)
}

// common returns the lowest common ancestor for which a feed update chunk is found in the chunk store
func (l *Finder) common(ctx context.Context, at, after int64) (*epoch, swarm.Chunk, error) {
	for e := lca(at, after); ; e = e.parent() {
		ch, err := l.get(ctx, e)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				if e.level == maxLevel {
					return e, nil, nil
				}
				continue
			}
			return e, nil, err
		}
		ts, err := updatedAt(ch)
		if err != nil {
			return e, nil, err
		}
		if ts <= uint64(at) {
			return e, ch, nil
		}
	}
}

// at is a non-concurrent recursive Finder function to find the version update chunk at time `at`
func (l *Finder) at(ctx context.Context, at uint64, e *epoch, ch swarm.Chunk) (swarm.Chunk, error) {
	// fmt.Printf("at=%d, epoch.start=%d, epoch.level=%d", at, e.start, e.level)
	uch, err := l.get(ctx, e)
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
		// fmt.Println("not found, trying left branch")
		return l.at(ctx, e.start-1, e.left(), ch)
	}
	// epoch found
	// check if timestamp is later then target
	ts, err := updatedAt(uch)
	if err != nil {
		return nil, err
	}
	// fmt.Printf("found, checking time: %d < %d\n", ts, at)
	if ts > at {
		if e.isLeft() {
			return ch, nil
		}
		return l.at(ctx, e.start-1, e.left(), ch)
	}
	if e.level == 0 { // matching update time or finest resolution
		return uch, nil
	}
	// continue traversing based on at
	return l.at(ctx, at, e.childAt(at), uch)
}

// FromChunk unwraps the content address chunk from the feed update soc
// it parses out the timestamp and the payload
func FromChunk(ch swarm.Chunk) ([]byte, uint64, error) {
	s, err := soc.FromChunk(ch)
	if err != nil {
		return nil, 0, err
	}
	cac := s.Chunk
	if len(cac.Data()) < 16 {
		return nil, 0, fmt.Errorf("feed update payload too short")
	}
	payload := cac.Data()[16:]
	at := binary.BigEndian.Uint64(cac.Data()[8:16])
	return payload, at, nil
}

func updatedAt(ch swarm.Chunk) (uint64, error) {
	_, ts, err := FromChunk(ch)
	if err != nil {
		return 0, err
	}
	return ts, nil
}
