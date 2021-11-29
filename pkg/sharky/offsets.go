package sharky

import (
	"os"
	"sync"
)

type freeSlots struct {
	sync.Mutex
	free  *offsetVector
	slots int64
	file  *os.File
}

func newOffsets(slots int64, freeOffsets []byte) *freeSlots {
	o := &freeSlots{
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
func (o *freeSlots) reclaim(offset int64) {
	o.Lock()
	o.free.unmark(offset)
	o.slots++
	o.Unlock()
}

func (o *freeSlots) next() (offset int64, rollback func()) {
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

func (o *freeSlots) rollbackF(offset int64) func() {
	return func() {
		o.Lock()
		defer o.Unlock()
		o.free.mark(offset)
		o.slots++
	}
}

type offsetVector struct {
	b    []byte
	bits int
	min  int64
}

func (o *offsetVector) grow() {
	o.bits++
	if len(o.b) < o.bits/8 {
		o.b = append(o.b, 0x00) // adds a new byte full of zeroes
	}
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
	if o.min > offset {
		o.min = offset
	}
	o.b[offset/8] = o.b[offset/8] ^ (1 << (offset % 8))
}

// unmark toggles the bit to false
func (o *offsetVector) unmark(offset int64) {
	o.b[offset/8] ^= (1 << (offset % 8))
	o.min = o.next()
}
