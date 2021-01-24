package feeds

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type Lookup interface {
	At(ctx context.Context, at, after int64) (swarm.Chunk, error)
}

// Getter encapsulates a chunk Getter getter and a feed and provides
//  non-concurrent lookup methods
type Getter struct {
	getter storage.Getter
	*Feed
}

// NewGetter constructs a feed Getter
func NewGetter(getter storage.Getter, feed *Feed) *Getter {
	return &Getter{getter, feed}
}

// Latest looks up the latest update of the feed
// after is a unix time hint of the latest known update
func Latest(ctx context.Context, l Lookup, after int64) (swarm.Chunk, error) {
	return l.At(ctx, time.Now().Unix(), after)
}

// Get creates an update of the underlying feed at the given epoch
// and looks it up in the chunk Getter based on its address
func (f *Getter) Get(ctx context.Context, i Index) (swarm.Chunk, error) {
	addr, err := f.Feed.Update(i).Address()
	if err != nil {
		return nil, err
	}
	return f.getter.Get(ctx, storage.ModeGetRequest, addr)
}

// it parses out the timestamp and the payload
func FromChunk(ch swarm.Chunk) (uint64, []byte, error) {
	s, err := soc.FromChunk(ch)
	if err != nil {
		return 0, nil, err
	}
	cac := s.Chunk
	if len(cac.Data()) < 16 {
		return 0, nil, fmt.Errorf("feed update payload too short")
	}
	payload := cac.Data()[16:]
	at := binary.BigEndian.Uint64(cac.Data()[8:16])
	return at, payload, nil
}

func UpdatedAt(ch swarm.Chunk) (uint64, error) {
	ts, _, err := FromChunk(ch)
	if err != nil {
		return 0, err
	}
	return ts, nil
}
