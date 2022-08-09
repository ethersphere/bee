// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky

import (
	"fmt"
	"io"
	"sync"
)

type slots struct {
	data []byte     // byteslice serving as bitvector: i-t bit set <>
	head uint32     // the first free slot
	file sharkyFile // file to persist free slots across sessions
	mtx  sync.Mutex
}

func newSlots(file sharkyFile) *slots {
	return &slots{
		file: file,
	}
}

func (sl *slots) load() error {
	data, err := io.ReadAll(sl.file)
	if err != nil {
		return err
	}
	sl.data = data
	return nil
}

// Save persists the free slot bitvector on disk (without closing)
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

	// new free slot is lower than the head
	if slot < sl.head {
		sl.head = slot
	}

	sl.data[slot/8] |= 1 << (slot % 8) // set bit to 1
}

// Use sets the slot as used.
func (sl *slots) Use(slot uint32) {

	sl.mtx.Lock()
	defer sl.mtx.Unlock()

	diff := int(slot/8) - len(sl.data)
	if diff >= 0 {
		fmt.Println("EXTENDING INSIDE USE")
		sl.extend(diff + 1)
	}

	sl.data[slot/8] &= ^(1 << (slot % 8)) // set bit to 0
}

// Next returns the lowest free slot.
func (sl *slots) Next() uint32 {
	sl.mtx.Lock()
	defer sl.mtx.Unlock()

	sl.head = sl.next()

	return sl.head
}

func (sl *slots) next() uint32 {
	for i := sl.head; i < sl.size(); i++ {
		if sl.data[i/8]&(1<<(i%8)) > 0 { // first 1 bit
			return i
		}
	}
	// no free slot was found, extend by 1 byte
	sl.extend(1)
	return sl.next()
}

// extend adapts the slots to an extended size shard
// extensions are bytewise: can only be multiples of 8 bits
func (sl *slots) extend(n int) {
	for i := 0; i < n; i++ {
		sl.data = append(sl.data, 0xff)
	}
}

// size in bits
func (sl *slots) size() uint32 {
	return uint32(len(sl.data) * 8)
}
