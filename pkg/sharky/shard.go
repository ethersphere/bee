// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
)

// LocationSize is the size of the byte representation of Location
const LocationSize int = 7

// Location models the location <shard, slot, length> of a chunk
type Location struct {
	Shard  uint8
	Slot   uint32
	Length uint16
}

func (l Location) String() string {
	return fmt.Sprintf("shard: %d, slot: %d, length: %d", l.Shard, l.Slot, l.Length)
}

// MarshalBinary returns byte representation of location
func (l *Location) MarshalBinary() ([]byte, error) {
	b := make([]byte, LocationSize)
	b[0] = l.Shard
	binary.LittleEndian.PutUint32(b[1:5], l.Slot)
	binary.LittleEndian.PutUint16(b[5:], l.Length)
	return b, nil
}

// UnmarshalBinary constructs the location from byte representation
func (l *Location) UnmarshalBinary(buf []byte) error {
	l.Shard = buf[0]
	l.Slot = binary.LittleEndian.Uint32(buf[1:5])
	l.Length = binary.LittleEndian.Uint16(buf[5:])
	return nil
}

// LocationFromBinary is a helper to construct a Location object from byte representation
func LocationFromBinary(buf []byte) (Location, error) {
	l := new(Location)
	err := l.UnmarshalBinary(buf)
	if err != nil {
		return Location{}, err
	}
	return *l, nil
}

// sharkyFile defines the minimal interface that is required for a file type for it to
// be usable in sharky. This allows us to have different implementations of file types
// that can continue using the sharky logic
type sharkyFile interface {
	io.ReadWriteCloser
	io.ReaderAt
	io.Seeker
	io.WriterAt
	Truncate(int64) error
	Sync() error
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

// read models the input to read operation (the output is an error)
type read struct {
	ctx  context.Context
	buf  []byte // variable size read buffer
	slot uint32 // slot to read from
}

// shard models a shard writing to a file with periodic offsets due to fixed maxDataSize
type shard struct {
	reads       chan read     // channel for reads
	errc        chan error    // result for reads
	writes      chan write    // channel for writes
	index       uint8         // index of the shard
	maxDataSize int           // max size of blobs
	file        sharkyFile    // the file handle the shard is writing data to
	slots       *slots        // component keeping track of freed slots
	quit        chan struct{} // channel to signal quitting
}

// forever loop processing
func (sh *shard) process() {
	var writes chan write
	var slot uint32
	defer func() {
		// this condition checks if an slot is in limbo (popped but not used for write op)
		if writes != nil {
			sh.slots.limboWG.Add(1)
			go func() {
				defer sh.slots.limboWG.Done()
				sh.slots.in <- slot
			}()
		}
	}()
	free := sh.slots.out

	for {
		select {
		case op := <-sh.reads:
			select {
			case sh.errc <- sh.read(op):
			case <-op.ctx.Done():
				// since the goroutine in the Read method can quit
				// on shutdown, we need to make sure that we can actually
				// write to the channel, since a shutdown is possible in
				// theory between after the point that the context is cancelled
				select {
				case sh.errc <- op.ctx.Err():
				case <-sh.quit:
					// since the Read method respects the quit channel
					// we can safely quit here without writing to the channel
					return
				}
			case <-sh.quit:
				return
			}

			// only enabled if there is a free slot previously popped
		case op := <-writes:
			op.res <- sh.write(op.buf, slot)
			free = sh.slots.out // re-enable popping a free slot next time we can write
			writes = nil        // disable popping a write operation until there is a free slot

			// pop a free slot
		case slot = <-free:
			// only if there is one can we pop a chunk to write otherwise keep back pressure on writes
			// effectively enforcing another shard to be chosen
			writes = sh.writes // enable popping a write operation
			free = nil         // disabling getting a new slot until a write is actually done

		case <-sh.quit:
			return
		}
	}
}

// close closes the shard:
// wait for pending operations to finish then saves free slots and blobs on disk
func (sh *shard) close() error {
	sh.slots.wg.Wait()
	if err := sh.slots.save(); err != nil {
		return err
	}
	if err := sh.slots.file.Close(); err != nil {
		return err
	}
	return sh.file.Close()
}

// offset calculates the offset from the slot
// this is possible since all blobs are of fixed size
func (sh *shard) offset(slot uint32) int64 {
	return int64(slot) * int64(sh.maxDataSize)
}

// read reads loc.Length bytes to the buffer from the blob slot loc.Slot
func (sh *shard) read(r read) error {
	n, err := sh.file.ReadAt(r.buf, sh.offset(r.slot))
	if err != nil {
		return fmt.Errorf("read %d: %w", n, err)
	}
	return nil
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
func (sh *shard) release(ctx context.Context, slot uint32) error {
	select {
	case sh.slots.in <- slot:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
