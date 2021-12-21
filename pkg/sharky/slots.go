// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky

import (
	"io"
	"os"
	"sync"
)

type slots struct {
	data  []byte         // byteslice serving as bitvector: i-t bit set <>
	size  uint32         // number of slots
	limit uint32         // max number of items in the shard
	head  uint32         // the first free slot
	file  *os.File       // file to persist free slots across sessions
	in    chan uint32    // incoming channel for free slots,
	out   chan uint32    // outgoing channel for free slots
	wg    sync.WaitGroup // signal releasing of free slots
}

func newSlots(file *os.File, limit uint32) *slots {
	return &slots{
		file:  file,
		limit: limit,
		in:    make(chan uint32),
		out:   make(chan uint32),
	}
}

// load inits the slots from file, called after init
func (sl *slots) load() (err error) {
	sl.data, err = io.ReadAll(sl.file)
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

// save persists the free slot bitvector on disk (without closing)
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
	return sl.file.Sync()
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
	sl.data[i/8] |= 1 << (i % 8)
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
	sl.data[head/8] &= ^(1 << (head % 8))
	sl.head = sl.next(head + 1)
	return head, head == sl.limit
}

// forever loop processing.
func (sl *slots) process(quit chan struct{}) {
	var head uint32     // the currently pending next free slots
	var out chan uint32 // nullable output channel, need to pop a free slot when nil
	var full bool       // set to true if uable to pop a free slot, i.e., head (zero value) is not a slot index
	for {
		// if out is nil, need to pop a new head unless quitting
		if out == nil && quit != nil {
			// if read a free slot to head, switch on case 0 by assigning out channel
			if head, full = sl.pop(); !full {
				out = sl.out
			}
		}

		select {
		// listen to released slots and append one to the slots
		case slot, more := <-sl.in:
			if !more {
				return
			}
			sl.push(slot)
			sl.wg.Done()

			// let out channel capture the free slot and set out to nil to pop a new free slot
		case out <- head:
			out = nil

			// quit is effective only after all initiated releases are received
		case <-quit:
			if out != nil {
				sl.push(head)
				out = nil
			}
			quit = nil
			sl.wg.Done()
			go func() {
				sl.wg.Wait()
				close(sl.in)
			}()
		}
	}
}
