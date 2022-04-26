package potter

import (
	"context"
	"encoding/binary"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ethersphere/bee/pkg/potter/persister"
	"github.com/ethersphere/bee/pkg/sharky"
)

var _ persister.LoadSaver = (*Sharky)(nil)

const (
	ShardCnt  = 8
	ShardSize = 1000000
	NodeSize  = 196
)

// Sharky wraps a sharky store to implement the persister.LoadSaver interface
type Sharky struct {
	*sharky.Store
}

func NewLoadSaver(dir string) (*Sharky, error) {
	s, err := sharky.New(&dirFS{basedir: dir}, ShardCnt, NodeSize)
	if err != nil {
		return nil, err
	}
	return &Sharky{s}, nil
}

func (s *Sharky) Load(ctx context.Context, ref []byte) ([]byte, error) {
	loc := Unmarshal(ref)
	buf := make([]byte, int(loc.Length))
	err := s.Store.Read(ctx, loc, buf)
	return buf, err
}

func (s *Sharky) Save(ctx context.Context, data []byte) ([]byte, error) {
	loc, err := s.Store.Write(ctx, data)
	if err != nil {
		return nil, err
	}
	return Marshal(loc), nil
}

func Marshal(loc sharky.Location) []byte {
	buf := make([]byte, 7)
	buf[0] = loc.Shard // uint8
	binary.BigEndian.PutUint32(buf[1:5], loc.Slot)
	binary.BigEndian.PutUint16(buf[5:7], loc.Length)
	return buf
}

func Unmarshal(buf []byte) (loc sharky.Location) {
	loc.Shard = buf[0] // uint8
	loc.Slot = binary.BigEndian.Uint32(buf[1:5])
	loc.Length = binary.BigEndian.Uint16(buf[5:7])
	return loc
}

type dirFS struct {
	basedir string
}

func (d *dirFS) Open(path string) (fs.File, error) {
	return os.OpenFile(filepath.Join(d.basedir, path), os.O_RDWR|os.O_CREATE, 0644)
}
