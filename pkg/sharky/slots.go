// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky

import (
	"io"
	"sync"
)

type slots struct {
	data   []byte             // byteslice serving as bitvector: i-t bit set <>
	limit  uint32             // max number of items in the shard
	head   uint32             // the first free slot
	file   sharkyFile         // file to persist free slots across sessions
	in     chan uint32        // incoming channel for free slots,
	out    chan uint32        // outgoing channel for free slots
	filled chan chan struct{} //
	wg     *sync.WaitGroup    // count started write operations
}

func newSlots(file sharkyFile, shardSize int, wg *sync.WaitGroup) *slots {
	return &slots{
		limit:  uint32(shardSize),
		file:   file,
		in:     make(chan uint32),
		out:    make(chan uint32),
		filled: make(chan chan struct{}),
		wg:     wg,
	}
}

// load inits the slots from file, called after init
func (sl *slots) load() (err error) {
	sl.data, err = io.ReadAll(sl.file)
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

// push inserts a free slot.
func (sl *slots) push(i uint32) {
	if sl.head > i {
		sl.head = i
	}
	sl.data[i/8] |= 1 << (i % 8)
}

// pop returns the lowest free slot.
func (sl *slots) pop() (slot uint32, full bool) {
	i := int(sl.head) / 8
	l := len(sl.data)
	// skip all zero bytes
	for ; i < l && sl.data[i] == 0x0; i++ {
	}
	// if we run out of free slots, increase data with an extra byte
	if i < l {
		// if there is a free slot
		j := 0
		// if no zero byte skipped, then find next bit
		if i == int(sl.head)/8 {
			j = int(sl.head) % 8
		}
		// must have a non-zero bit
		for ; j < 8 && sl.data[i]&(1<<j) == 0; j++ {
		}
		slot = uint32(i*8 + j)
		sl.head = slot + 1
		return slot, false
	}
	// if shard size limit is reached
	left := int(sl.limit) - l*8
	if left <= 0 {
		sl.head = uint32(l) * 8
		return 0, true
	}
	// the byte appended to contains free slots upto what the shard size allows
	var tail uint8 = 255
	if left < 8 {
		tail = uint8(1<<left - 1)
	}
	sl.data = append(sl.data, tail)
	// increment head cursor
	sl.head = uint32(i)*8 + 1
	// return free slot index (the slot bit is not unset elsewhere)
	return uint32(i * 8), false
}

// forever loop processing.
func (sl *slots) process(quit chan struct{}) {
	var slot uint32     // the currently pending next free slots
	var out chan uint32 // nullable output channel, need to pop a free slot when nil
	need := true        //
	var full, quitting bool
	for {
		if need && !quitting {
			// if read a free slot to head, switch to another case  by assigning out channel
			slot, full = sl.pop()
			if !full {
				need = false
				out = sl.out
			}
		}
		select {
		case c := <-sl.filled:
			// if out is nil, need to pop a new head unless quitting
			sl.data[slot/8] &= ^(1 << (slot % 8))
			c <- struct{}{}
			need = true

			// listen to released slots and append one to the slots
		case free, more := <-sl.in:
			if !more {
				return
			}
			sl.push(free)
			need = full

			// let the out channel capture the free slot and set out to nil to pop a new free slot
		case out <- slot:
			out = nil
			quit = nil

			// quit is effective only after all initiated releases are received
		case <-quit:
			quitting = true
			quit = nil
			close(sl.in)
		}
	}
}
