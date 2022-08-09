// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky

import (
	"encoding/binary"
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

// shard models a shard writing to a file with periodic offsets due to fixed maxDataSize
type shard struct {
	available   chan availableShard
	index       uint8         // index of the shard
	maxDataSize int           // max size of blobs
	file        sharkyFile    // the file handle the shard is writing data to
	slots       *slots        // component keeping track of freed slots
	quit        chan struct{} // channel to signal quitting
}

// forever loop processing
func (sh *shard) process() {
	defer sh.close()

	for {
		slot := sh.slots.Next()
		select {
		case sh.available <- availableShard{shard: sh.index, slot: slot}:
			sh.slots.Use(slot)
		case <-sh.quit:
			return
		}
	}
}

// close closes the shard:
// wait for pending operations to finish then saves free slots and blobs on disk
func (sh *shard) close() error {
	if err := sh.slots.Save(); err != nil {
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
func (sh *shard) read(buf []byte, slot uint32) error {
	_, err := sh.file.ReadAt(buf, sh.offset(slot))
	return err
}

// write writes loc.Length bytes to the buffer from the blob slot loc.Slot
func (sh *shard) write(buf []byte, slot uint32) (Location, error) {
	n, err := sh.file.WriteAt(buf, sh.offset(slot))
	return Location{
		Shard:  sh.index,
		Slot:   slot,
		Length: uint16(n),
	}, err
}

// release frees the slot allowing new entry to overwrite
func (sh *shard) release(slot uint32) {
	sh.slots.Free(slot)
}
