// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky

import (
	"errors"
	"io"
)

var errFull = errors.New("full")

type slots struct {
	data []byte     // byteslice serving as bitvector: i-t bit set <>
	head uint32     // the first free slot
	size uint32     // the first free slot
	file sharkyFile // file to persist free slots across sessions
}

func newSlots(file sharkyFile, size uint32) *slots {
	return &slots{
		file: file,
		size: size,
	}
}

// load inits the slots from file, called after init
func (sl *slots) load() error {
	tmp, err := io.ReadAll(sl.file)
	if err != nil {
		return err
	}
	sl.data = tmp

	// if the size of the loaded file is smaller than the desired size
	// then set the trailing bits as free slots
	for i := len(sl.data) * 8; i < int(sl.size); i += 8 {
		sl.data = append(sl.data, 0xff)
	}

	sl.head = sl.next(0)
	return nil
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

func (sl *slots) rewindHead(head uint32) {
	if head < sl.head {
		sl.head = head
	}
}

// pop returns the lowest available free slot.
func (sl *slots) pop() (uint32, error) {
	head := sl.head
	if head >= sl.size {
		return 0, errFull
	}
	sl.data[head/8] &= ^(1 << (head % 8)) // set bit to 0
	sl.head = sl.next(head + 1)
	return head, nil
}

// next returns the lowest free slot after start.
func (sl *slots) next(start uint32) uint32 {
	for i := start; i < sl.size; i++ {
		if sl.data[i/8]&(1<<(i%8)) > 0 { // first 1 bit
			return i
		}
	}
	return sl.size
}
