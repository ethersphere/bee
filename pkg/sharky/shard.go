// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"sync"
)

// location models the location <shard, offset, length> of a chunk
type Location struct {
	Shard  uint8
	Offset int64
	Length int64
}

func (l *Location) MarshalBinary() ([]byte, error) {
	b := make([]byte, 1+binary.MaxVarintLen64*2)
	b[0] = l.Shard
	binary.PutVarint(b[1:1+binary.MaxVarintLen64], l.Offset)
	binary.PutVarint(b[1+binary.MaxVarintLen64:], l.Length)
	return b, nil
}

func (l *Location) UnmarshalBinary(buf []byte) error {
	l.Shard = buf[0]
	l.Offset, _ = binary.Varint(buf[1 : 1+binary.MaxVarintLen64])
	l.Length, _ = binary.Varint(buf[1+binary.MaxVarintLen64:])
	return nil
}

func LocationFromBinary(buf []byte) (*Location, error) {
	l := &Location{}
	err := l.UnmarshalBinary(buf)
	if err != nil {
		return nil, err
	}
	return l, nil
}

// operation models both read and write
type operation struct {
	location Location   // shard, offset, length combo - given for read, returned by write
	buffer   []byte     // fixed size byte slice for data - copied to by read
	data     []byte     // variable length byte slice - given for write
	err      chan error // signal for end of operation
}

// shard models a shard writing to a file with periodic offsets due to fixed datasize
type shard struct {
	readOps  chan *operation // channel for reads
	writeOps chan *operation // channel for writes
	free     chan int64      // channel for offsets available to write
	freed    chan int64      // channel for offsets freed by garbage collection
	index    uint8           // index of the shard
	limit    int64           // max number of items in the shard
	fh       *os.File        // the file handle the shard is writing data to
	ffh      *os.File        // the file handle the shard is writing free slats to
	quit     chan struct{}   // channel to signal quitting
	wg       *sync.WaitGroup // waitgroup to allow clean closing
}

// forever loop processing
func (sh *shard) offer(size int64) {
	var offset int64
	defer sh.wg.Done()
	for {
		// first try to obtain a free offset from among freed ones
		select {
		case offset = <-sh.freed:
		case <-sh.quit:
			return
			// fallback to extending shard until limit
		default:
			// if limit is reached we only allow free offsets via release
			if size == sh.limit {
				select {
				case offset = <-sh.freed:
				case <-sh.quit:
					return
				}
			} else {
				// shard is allowed to grow upto limit
				offset = size * DataSize
				size++
			}
		}
		// push the obtained offset to the main process
		select {
		case sh.free <- offset:
		case <-sh.quit:
			// remember free offset in limbo
			sh.freed <- offset
			return
		}
	}
}

// forever loop processing
func (sh *shard) process() {
	var writeOps chan *operation
	var offset int64
	free := sh.free
	defer sh.wg.Done()
	for {
		select {
		// prioritise read ops
		case op := <-sh.readOps:
			sh.read(op)
		case <-sh.quit:
			// this condition checks if an offset is in limbo (popped but not used for write op)
			if writeOps != nil {
				sh.freed <- offset
			}
			return
		default:
			select {

			// pop a free offset
			case offset = <-free:
				// only if there is one can we pop a chunk to write otherwise keep back pressure on writes
				// effectively enforcing another shard to be chosen
				writeOps = sh.writeOps // enable popping a write operation
				free = nil             // disabling getting a new position until a write is actually done

				// only enabled if there is a free offset (pos) previously popped
			case op := <-writeOps:
				sh.write(op, offset)
				free = sh.free // reenable popping a free slot next time we can write
				writeOps = nil // disable popping a write operation until there is a free slot

				// prioritise read operations
			case op := <-sh.readOps:
				sh.read(op)

			case <-sh.quit:
				// again put back offset in limbo
				if writeOps != nil {
					sh.freed <- offset
				}
				return
			}
		}

	}
}

func (sh *shard) close() error {
	go func() { // close free channel only after all relevant inflight change
		sh.wg.Wait()
		close(sh.freed)
	}()
	free := []int64{}
	for offset := range sh.freed {
		free = append(free, offset)
	}
	frees, err := json.Marshal(free)
	if err != nil {
		return err
	}
	if err = sh.ffh.Truncate(0); err != nil {
		return err
	}
	if _, err = sh.ffh.Seek(0, 0); err != nil {
		return err
	}
	if _, err := sh.ffh.Write(frees); err != nil {
		return err
	}
	if err := sh.ffh.Close(); err != nil {
		return err
	}
	return sh.fh.Close()
}

// not  called concurrently
func (sh *shard) read(op *operation) {
	_, err := sh.fh.ReadAt(op.buffer[:op.location.Length], op.location.Offset)
	select {
	case op.err <- err:
	case <-sh.quit:
	}
}

// not  called concurrently
func (sh *shard) write(op *operation, offset int64) {
	sh.wg.Add(1)
	_, err := sh.fh.WriteAt(op.data, offset)
	if err != nil {
		go func() {
			sh.freed <- offset
			sh.wg.Done()
		}()
	} else {
		defer sh.wg.Done()
		op.location.Offset = offset
		op.location.Shard = sh.index
		op.location.Length = int64(len(op.data))
	}
	select {
	case op.err <- err:
	case <-sh.quit:
	}
}

func (sh *shard) release(offset int64) {
	sh.wg.Add(1)
	go func() {
		sh.freed <- offset
		sh.wg.Done()
	}()
}
