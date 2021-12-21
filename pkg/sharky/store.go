// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// # lockless sharding

// * shard choice responding to backpressure by running operation
// * read prioritisation over writing
// * free slots allow write

package sharky

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"sync"

	"github.com/hashicorp/go-multierror"
)

var (
	// Error returned by Write if the blob length exceeds the max blobsize
	ErrTooLong = errors.New("data too long")
)

// Store models the sharded fix-length blobstore
type Store struct {
	datasize int           // max length of blobs
	pool     *sync.Pool    // pool to save on allocating channels
	writes   chan write    // shared write operations channel
	shards   []*shard      // shards
	quit     chan struct{} // quit channel
}

// New constructs a sharded blobstore
// arguments:
// - base directory string
// - shard count - positive integer < 256 - cannot be zero or expect panic
// - shard size - positive integer multiple of 8 - for others expect undefined behaviour
// - datasize - positive integer representing the maximum blob size to be stored
func New(basedir string, shardCnt int, limit uint32, datasize int) (*Store, error) {
	pool := &sync.Pool{New: func() interface{} {
		return make(chan entry)
	}}
	sh := &Store{
		datasize: datasize,
		pool:     pool,
		writes:   make(chan write),
		shards:   make([]*shard, shardCnt),
		quit:     make(chan struct{}),
	}
	for i := range sh.shards {
		s, err := sh.create(uint8(i), limit, datasize, basedir)
		if err != nil {
			return nil, err
		}
		sh.shards[i] = s
	}
	return sh, nil
}

// Close closes each shard and return incidental errors from each shard
func (s *Store) Close() (err error) {
	close(s.quit)
	for _, sh := range s.shards {
		err = multierror.Append(err, sh.close())
	}
	return err.(*multierror.Error).ErrorOrNil()
}

// create creates a new shard with index, max capacity limit, file within base directory
func (s *Store) create(index uint8, limit uint32, datasize int, basedir string) (*shard, error) {
	file, err := os.OpenFile(path.Join(basedir, fmt.Sprintf("shard_%03d", index)), os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return nil, err
	}
	ffile, err := os.OpenFile(path.Join(basedir, fmt.Sprintf("free_%03d", index)), os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return nil, err
	}
	sl := newSlots(ffile, limit)
	err = sl.load()
	if err != nil {
		return nil, err
	}
	sh := &shard{
		reads:    make(chan read),
		errc:     make(chan error),
		writes:   s.writes,
		index:    index,
		datasize: datasize,
		file:     file,
		slots:    sl,
		quit:     s.quit,
	}
	terminated := make(chan struct{})
	sh.slots.wg.Add(1)
	go func() {
		sh.process()
		close(terminated)
	}()
	go sl.process(terminated)
	return sh, nil
}

// Read reads the content of the blob found at location into the byte buffer given
// The location is assumed to be obtained by an earlier Write call storing the blob
func (s *Store) Read(ctx context.Context, loc Location, buf []byte) error {
	sh := s.shards[loc.Shard]
	select {
	case sh.reads <- read{buf[:loc.Length], loc.Slot, 0}:
	case <-ctx.Done():
		return ctx.Err()
	}
	return <-sh.errc
}

// Write stores a new blob and returns its location to be used as a reference
// It can be given to a Read call to return the stored blob.
func (s *Store) Write(ctx context.Context, data []byte) (loc Location, err error) {
	if len(data) > s.datasize {
		return loc, ErrTooLong
	}
	c := s.pool.Get().(chan entry)
	defer s.pool.Put(c)
	select {
	case s.writes <- write{data, c}:
	case <-ctx.Done():
		return loc, ctx.Err()
	}

	e := <-c
	return e.loc, e.err
}

// Release gives back the slot to the shard
// From here on the slot can be reused and overwritten
// Release is meant to be called when an entry in the upstream db is removed
// Note that releasing is not safe for obfuscating earlier content, since
// even after reuse, the slot may be used by a very short blob and leaves the
// rest of the old blob bytes untouched
func (s *Store) Release(loc Location) {
	sh := s.shards[loc.Shard]
	sh.slots.wg.Add(1)
	go sh.release(loc.Slot)
}
