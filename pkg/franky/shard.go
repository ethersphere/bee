package franky

import (
	"fmt"
	"os"
	"path"
	"sync"

	"github.com/ethersphere/bee/pkg/swarm"
)

type shard struct {
	sync.Mutex
	f *os.File

	slots *offsets
}

func newShard(id int, capacity int64, chunkSize int64, basepath string, freeOffsets []byte) (*shard, error) {
	fh, err := os.OpenFile(fname(id, basepath), os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	fi, err := fh.Stat()
	if err != nil {
		return nil, err
	}
	if fi.Size() == 0 {
		// preallocate the shard
		err = fh.Truncate(capacity * chunkSize)
		if err != nil {
			return nil, err
		}
	}

	s := &shard{f: fh, slots: newOffsets(capacity, freeOffsets)}
	return s, nil
}

func (s *shard) readAt(offset int64) ([]byte, error) {
	s.Lock()
	defer s.Unlock()

	o := int64(offset * swarm.ChunkWithSpanSize)
	data := make([]byte, swarm.ChunkWithSpanSize)
	if _, err := s.f.ReadAt(data, o); err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return data, nil
}

func (s *shard) writeAt(data []byte, offset int64) error {
	s.Lock()
	defer s.Unlock()

	o := int64(offset * swarm.ChunkWithSpanSize)
	if _, err := s.f.WriteAt(data, o); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

func (s *shard) freeOffset() (int64, func()) {
	return s.slots.next()
}

func (s *shard) release(offset int64) {
	s.slots.reclaim(offset)
}

func (s *shard) offsetBytes() []byte {
	return s.slots.free.b
}

func (s *shard) close() ([]byte, error) {
	s.Lock()
	defer s.Unlock()
	return s.offsetBytes(), s.f.Close()
}
func fname(id int, base string) string {
	return path.Join(base, fmt.Sprintf("shard_%03d", id))
}

type offsets struct {
	sync.Mutex
	free  *offsetVector
	dirty *offsetVector
	slots int64
	file  *os.File
}

func newOffsets(slots int64, freeOffsets []byte) *offsets {
	o := &offsets{
		dirty: &offsetVector{b: make([]byte, slots/8)},
		slots: slots,
	}

	if freeOffsets == nil {
		// slots need to be a power of 2
		freeOffsets = make([]byte, slots/8)
		for i := range freeOffsets {
			freeOffsets[i] = 0xff
		}
	}
	if len(freeOffsets) != int(slots/8) {
		panic("offset mismatch")
	}
	o.free = &offsetVector{b: freeOffsets}

	return o
}
func (o *offsets) reclaim(offset int64) {
	o.Lock()
	o.free.unmark(offset)
	o.slots++
	o.Unlock()
}

func (o *offsets) next() (offset int64, rollback func()) {
	o.Lock()
	defer o.Unlock()
	if o.slots == 0 {
		return -1, nil
	}

	for {
		offset = o.free.next()
		if offset == -1 {
			// no free offsets
			return offset, nil
		}
		o.free.unmark(offset)
		o.slots--
		return offset, o.rollbackF(offset)
	}
}

func (o *offsets) rollbackF(offset int64) func() {
	return func() {
		o.Lock()
		defer o.Unlock()
		o.free.mark(offset)
		o.slots++
	}
}

type offsetVector struct {
	b []byte
}

// next returns the next free offset.
func (o *offsetVector) next() int64 {
	for i := 0; i < len(o.b); i++ {
		for j := 0; j < 8; j++ {
			if o.b[i]&(1<<j) > 0 {
				return int64(i*8 + j)
			}
		}
	}
	return int64(-1)
}

func (o *offsetVector) set(offset int64) bool {
	return o.b[offset/8]&(1<<(offset%8)) > 0
}

// mark toggles the bit to true
func (o *offsetVector) mark(offset int64) {
	o.b[offset/8] = o.b[offset/8] ^ (1 << (offset % 8))
}

// unmark toggles the bit to false
func (o *offsetVector) unmark(offset int64) {
	o.b[offset/8] ^= (1 << (offset % 8))
}
