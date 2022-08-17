// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky

import (
	"io"
	"sync"
)

type slots struct {
	data []byte     // byteslice serving as bitvector: i-t bit set <>
	head uint32     // the first free slot
	size uint32     // number of slots
	file sharkyFile // file to persist free slots across sessions
	mtx  sync.Mutex
}

func newSlots(file sharkyFile) *slots {
	return &slots{
		file: file,
	}
}

// load inits the slots from file, called after init
func (sl *slots) load() (err error) {

	sl.mtx.Lock()
	defer sl.mtx.Unlock()

	sl.data, err = io.ReadAll(sl.file)
	if err != nil {
		return err
	}
	sl.size = uint32(len(sl.data) * 8)
	return err
}

// save persists the free slot bitvector on disk (without closing)
func (sl *slots) Save() error {

	sl.mtx.Lock()
	defer sl.mtx.Unlock()

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

func (sl *slots) Free(slot uint32) {

	sl.mtx.Lock()
	defer sl.mtx.Unlock()

	if slot < sl.head {
		sl.head = slot
	}
	sl.data[slot/8] |= 1 << (slot % 8) // set bit to 1
}

// pop sets the head as used, and finds the next free slot.
func (sl *slots) Use(slot uint32) {

	sl.mtx.Lock()
	defer sl.mtx.Unlock()

	if sl.data[slot/8]&(1<<(slot%8)) == 0 {
		panic("lol")
	}

	sl.data[slot/8] &= ^(1 << (slot % 8)) // set bit to 0
}

// next returns the lowest free slot.
func (sl *slots) Next() uint32 {
	sl.mtx.Lock()
	defer sl.mtx.Unlock()

	sl.head = sl.next()

	if sl.data[sl.head/8]&(1<<(sl.head%8)) == 0 {
		panic("lol")
	}

	return sl.head
}

func (sl *slots) next() uint32 {
	for i := sl.head; i < sl.size; i++ {
		if sl.data[i/8]&(1<<(i%8)) > 0 { // first 1 bit
			return i
		}
	}
	// extend
	sl.size += 8
	sl.data = append(sl.data, 0xff)
	return sl.size - 8
}
