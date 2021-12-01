// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky

import (
	"os"
)

// location models the location <shard, slot, length> of a chunk
type Location struct {
	Shard  uint8
	Slot   uint32
	Length uint16
}

// operation models both read and write
type operation struct {
	location Location   // shard, slot, length combo - given for read, returned by write
	buffer   []byte     // fixed size byte slice for data - copied to by read
	data     []byte     // variable length byte slice - given for write
	err      chan error // signal for end of operation
}

// shard models a shard writing to a file with periodic offsets due to fixed datasize
type shard struct {
	readOps  chan *operation // channel for reads
	writeOps chan *operation // channel for writes
	slots    *slots          // component keeping track of freed slots
	index    uint8           // index of the shard
	file     *os.File        // the file handle the shard is writing data to
	quit     chan struct{}   // channel to signal quitting
}

// forever loop processing
func (sh *shard) process() {
	var writeOps chan *operation
	var slot uint32
	free := sh.slots.out
	for {
		select {
		// prioritise read ops
		case op := <-sh.readOps:
			sh.read(op)
		case <-sh.quit:
			// this condition checks if an slot is in limbo (popped but not used for write op)
			// this select case is not necessary
			if writeOps != nil {
				sh.release(slot)
			}
			return
		default:
			select {
			// pop a free slot
			case slot = <-free:
				// only if there is one can we pop a chunk to write otherwise keep back pressure on writes
				// effectively enforcing another shard to be chosen
				writeOps = sh.writeOps // enable popping a write operation
				free = nil             // disabling getting a new position until a write is actually done

				// only enabled if there is a free slot previously popped
			case op := <-writeOps:
				sh.write(op, slot)
				free = sh.slots.out // reenable popping a free slot next time we can write
				writeOps = nil      // disable popping a write operation until there is a free slot

				// prioritise read operations
			case op := <-sh.readOps:
				sh.read(op)

			case <-sh.quit:
				// again put back slot in limbo
				if writeOps != nil {
					sh.release(slot)
				}
				return
			}
		}

	}
}

func (sh *shard) close() error {
	if err := sh.slots.save(); err != nil {
		return err
	}
	return sh.file.Close()
}

func offset(slot uint32) int64 {
	return int64(slot) * DataSize
}

// not  called concurrently
func (sh *shard) read(op *operation) {
	_, err := sh.file.ReadAt(op.buffer[:op.location.Length], offset(op.location.Slot))
	select {
	case op.err <- err:
	case <-sh.quit:
	}
}

// not  called concurrently
func (sh *shard) write(op *operation, slot uint32) {
	_, err := sh.file.WriteAt(op.data, offset(slot))
	// fmt.Printf("write. shard: %d, slot: %d, offset: %d, length %d, data: %x, slots size: %d, slots limit: %d, head: %d\n", sh.index, slot, offset(slot), len(op.data), op.data,: sh.slots.size, sh.slots.limit, sh.slots.head)
	if err != nil {
		sh.release(slot)
	} else {
		op.location.Slot = slot
		op.location.Shard = sh.index
		op.location.Length = uint16(len(op.data))
	}
	select {
	case op.err <- err:
	case <-sh.quit:
	}
}

func (sh *shard) release(slot uint32) {
	select {
	case sh.slots.in <- slot:
	case sh.slots.restore <- slot:
	}
}
