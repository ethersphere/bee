package indigo

import (
	"context"
	"encoding/binary"

	"github.com/ethersphere/bee/pkg/indigo/persister"
	"github.com/ethersphere/bee/pkg/sharky"
)

var shardCnt = 8

var _ persister.LoadSaver = (*Sharky)(nil)

// Sharky wraps a sharky store to implement the persister.LoadSaver interface
type Sharky struct {
	*sharky.Shards
}

func NewLoadSaver(dir string) (*Sharky, error) {
	s, err := sharky.New(dir, shardCnt, 0)
	if err != nil {
		return nil, err
	}
	return &Sharky{s}, nil
}

func (s *Sharky) Load(ctx context.Context, ref []byte) ([]byte, error) {
	return s.Shards.Read(ctx, Unmarshal(ref))
}

func (s *Sharky) Save(ctx context.Context, data []byte) ([]byte, error) {
	loc, err := s.Shards.Write(ctx, data)
	if err != nil {
		return nil, err
	}
	return Marshal(loc), nil
}

func Marshal(loc sharky.Location) []byte {
	buf := make([]byte, 8)
	buf[0] = loc.Shard // uint8
	binary.BigEndian.PutUint32(buf, uint32(loc.Length))
	binary.BigEndian.PutUint32(buf[4:], uint32(loc.Offset))
	return buf
}

func Unmarshal(buf []byte) (loc sharky.Location) {
	loc.Shard = buf[0] // uint8
	buf[0] = 0
	loc.Length = int64(binary.BigEndian.Uint32(buf[:4]))
	loc.Offset = int64(binary.BigEndian.Uint32(buf[4:]))
	return loc
}
