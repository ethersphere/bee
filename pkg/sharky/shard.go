// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky

import (
	"os"
)

// Location models the location <shard, slot, length> of a chunk
type Location struct {
	Shard  uint8
	Slot   uint32
	Length uint16
}

// write models the input to a write operation
type write struct {
	buf []byte     // variable size read buffer
	res chan entry // to put the result through
}

// entry models the output result of a write operation
type entry struct {
	loc Location // shard, slot, length combo
	err error    // signal for end of operation
}

// read models the input to read opeeration (the output is an error)
type read struct {
	buf  []byte // variable size read buffer
	slot uint32 // slot to read from
	at   int    // within-chunk offset for partial reads
}

// shard models a shard writing to a file with periodic offsets due to fixed datasize
type shard struct {
	reads    chan read     // channel for reads
	errc     chan error    // result for reads
	writes   chan write    // channel for writes
	index    uint8         // index of the shard
	datasize int           // max size of blobs
	file     *os.File      // the file handle the shard is writing data to
	slots    *slots        // component keeping track of freed slots
	quit     chan struct{} // channel to signal quitting
}

// forever loop processing
func (sh *shard) process() {
	var writes chan write
	var slot uint32
	free := sh.slots.out
LOOP:
	for {
		select {
		case op := <-sh.reads:
			// prioritise read ops
			var reads chan read
			for {
				select {
				case op = <-reads:
				default:
					if reads != nil {
						continue LOOP
					}
				}
				reads = sh.reads
				sh.errc <- sh.read(op)
			}

			// only enabled if there is a free slot previously popped
		case op := <-writes:
			op.res <- sh.write(op.buf, slot)
			free = sh.slots.out // reenable popping a free slot next time we can write
			writes = nil        // disable popping a write operation until there is a free slot

			// pop a free slot
		case slot = <-free:
			// only if there is one can we pop a chunk to write otherwise keep back pressure on writes
			// effectively enforcing another shard to be chosen
			writes = sh.writes // enable popping a write operation
			free = nil         // disabling getting a new slot until a write is actually done

		case <-sh.quit:
			// this condition checks if an slot is in limbo (popped but not used for write op)
			if writes != nil {
				sh.release(slot)
			}
			return
		}
	}
}

// close closes the shard:
// wait for pending operations to finish then saves free slots and blobs on disk
func (sh *shard) close() error {
	<-sh.slots.stopped
	if err := sh.slots.save(); err != nil {
		return err
	}
	if err := sh.slots.close(); err != nil {
		return err
	}
	return sh.file.Close()
}

// offset calculates the offset from the slot
// this is possible since all blobs are of fixed size
func (sh *shard) offset(slot uint32) int64 {
	return int64(slot) * int64(sh.datasize)
}

// read reads loc.Length bytes to the buffer from the blob slot loc.Slot
func (sh *shard) read(r read) error {
	_, err := sh.file.ReadAt(r.buf, sh.offset(r.slot)+int64(r.at))
	return err
}

// write writes loc.Length bytes to the buffer from the blob slot loc.Slot
func (sh *shard) write(buf []byte, slot uint32) entry {
	n, err := sh.file.WriteAt(buf, sh.offset(slot))
	return entry{
		loc: Location{
			Shard:  sh.index,
			Slot:   slot,
			Length: uint16(n),
		},
		err: err,
	}
}

// release frees the slot allowing new entry to overwrite
func (sh *shard) release(slot uint32) {
	select {
	case sh.slots.in <- slot:
	case sh.slots.rest <- slot:
	}
}
