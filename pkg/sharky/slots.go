package sharky

import (
	"io/ioutil"
	"os"
)

type slots struct {
	data  []byte      // byteslice serving as bitvector: i-t bit set <>
	size  uint32      // number of slots
	limit uint32      // max number of items in the shard
	head  uint32      // the first free slot
	file  *os.File    // file to persist free slots across sessions
	in    chan uint32 // incoming channel for free slots,
	out   chan uint32 // outgoing channel for free slots
	rest  chan uint32 // channel for freeing popped slots not yet written to
}

func newSlots(size uint32, file *os.File, limit uint32) *slots {
	return &slots{
		size:  size,
		file:  file,
		limit: limit,
		in:    make(chan uint32),
		out:   make(chan uint32),
		rest:  make(chan uint32),
	}
}

// load inits the slots from file, called after init
func (sl *slots) load() (err error) {
	sl.data, err = ioutil.ReadAll(sl.file)
	if err != nil {
		return err
	}
	sl.size = uint32(len(sl.data) * 8)
	if sl.size > sl.limit {
		sl.size = sl.limit
	}
	sl.head = sl.next(0)
	return err
}

// save persists the free slot bitvector on disk
func (sl *slots) save() error {
	if err := sl.file.Truncate(0); err != nil {
		return err
	}
	if _, err := sl.file.Seek(0, 0); err != nil {
		return err
	}
	if _, err := sl.file.Write(sl.data); err != nil {
		return err
	}
	if err := sl.file.Sync(); err != nil {
		return err
	}
	return sl.file.Close()
}

// extend adapts the slots to an extended size shard
// extensions are bytewise: can only be multiples of 8 bits
func (sl *slots) extend(n int) {
	sl.size += uint32(n) * 8
	if sl.size > sl.limit {
		sl.size = sl.limit
	}
	for i := 0; i < n; i++ {
		sl.data = append(sl.data, 0xff)
	}
}

// next returns the lowest free slot after start.
func (sl *slots) next(start uint32) uint32 {
	for i := start; i < sl.size; i++ {
		if sl.data[i/8]&(1<<(i%8)) > 0 {
			return i
		}
	}
	return sl.size
}

// push inserts a free slot.
func (sl *slots) push(i uint32) {
	if sl.head > i {
		sl.head = i
	}
	sl.data[i/8] |= (1 << (i % 8))
}

// pop returns the lowest available free slot.
func (sl *slots) pop() (uint32, bool) {
	head := sl.head
	if head == sl.limit {
		return head, true
	}
	if head == sl.size && sl.size < sl.limit {
		sl.extend(1)
	}
	sl.data[head/8] &= (0xff ^ (1 << (head % 8)))
	sl.head = sl.next(head + 1)
	return head, head == sl.limit
}

// forever loop processing.
func (sl *slots) process(quit chan struct{}) {
	defer func() {
		close(sl.rest)
		for slot := range sl.rest {
			sl.push(slot)
		}
	}()
	var out chan uint32
	var full bool
	var head uint32
	for {
		if out == nil {
			head, full = sl.pop()
			if !full {
				out = sl.out
			}
		}
		select {
		case slot := <-sl.in:
			sl.push(slot)
		case out <- head:
			out = nil
		case <-quit:
			if !full {
				sl.push(head)
			}
			return
		}
	}
}
