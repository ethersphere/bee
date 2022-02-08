// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"sync"

	"github.com/hashicorp/go-multierror"
)

var (
	// ErrTooLong returned by Write if the blob length exceeds the max blobsize.
	ErrTooLong = errors.New("data too long")
	// ErrQuitting returned by Write when the store is Closed before the write completes.
	ErrQuitting = errors.New("quitting")
)

// Store models the sharded fix-length blobstore
// Design provides lockless sharding:
// - shard choice responding to backpressure by running operation
// - read prioritisation over writing
// - free slots allow write
type Store struct {
	maxDataSize int             // max length of blobs
	writes      chan write      // shared write operations channel
	shards      []*shard        // shards
	wg          *sync.WaitGroup // count started operations
	quit        chan struct{}   // quit channel
	metrics     metrics
}

// New constructs a sharded blobstore
// arguments:
// - base directory string
// - shard count - positive integer < 256 - cannot be zero or expect panic
// - shard size - positive integer multiple of 8 - for others expect undefined behaviour
// - maxDataSize - positive integer representing the maximum blob size to be stored
func New(basedir fs.FS, shardCnt int, maxDataSize int) (*Store, error) {
	store := &Store{
		maxDataSize: maxDataSize,
		writes:      make(chan write),
		shards:      make([]*shard, shardCnt),
		wg:          &sync.WaitGroup{},
		quit:        make(chan struct{}),
		metrics:     newMetrics(),
	}
	for i := range store.shards {
		s, err := store.create(uint8(i), maxDataSize, basedir)
		if err != nil {
			return nil, err
		}
		store.shards[i] = s
	}
	store.metrics.ShardCount.Set(float64(len(store.shards)))

	return store, nil
}

// Close closes each shard and return incidental errors from each shard
func (s *Store) Close() error {
	close(s.quit)
	err := new(multierror.Error)
	for _, sh := range s.shards {
		err = multierror.Append(err, sh.close())
	}

	return err.ErrorOrNil()
}

// create creates a new shard with index, max capacity limit, file within base directory
func (s *Store) create(index uint8, maxDataSize int, basedir fs.FS) (*shard, error) {
	file, err := basedir.Open(fmt.Sprintf("shard_%03d", index))
	if err != nil {
		return nil, err
	}
	ffile, err := basedir.Open(fmt.Sprintf("free_%03d", index))
	if err != nil {
		return nil, err
	}
	sl := newSlots(ffile.(sharkyFile), s.wg)
	err = sl.load()
	if err != nil {
		return nil, err
	}
	sh := &shard{
		reads:       make(chan read),
		errc:        make(chan error),
		writes:      s.writes,
		index:       index,
		maxDataSize: maxDataSize,
		file:        file.(sharkyFile),
		slots:       sl,
		quit:        s.quit,
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
func (s *Store) Read(ctx context.Context, loc Location, buf []byte) (err error) {
	sh := s.shards[loc.Shard]
	select {
	case sh.reads <- read{buf[:loc.Length], loc.Slot}:
		s.metrics.TotalReadCalls.Inc()
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case err = <-sh.errc:
		if err != nil {
			s.metrics.TotalReadCallsErr.Inc()
		}
		return err
	case <-s.quit:
		return ErrQuitting
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Write stores a new blob and returns its location to be used as a reference
// It can be given to a Read call to return the stored blob.
func (s *Store) Write(ctx context.Context, data []byte) (loc Location, err error) {
	if len(data) > s.maxDataSize {
		return loc, ErrTooLong
	}
	s.wg.Add(1)
	defer s.wg.Done()

	c := make(chan entry)

	select {
	case s.writes <- write{data, c}:
		s.metrics.TotalWriteCalls.Inc()
	case <-s.quit:
		return loc, ErrQuitting
	case <-ctx.Done():
		return loc, ctx.Err()
	}

	select {
	case e := <-c:
		if e.err == nil {
			shard := strconv.Itoa(int(e.loc.Shard))
			s.metrics.CurrentShardSize.WithLabelValues(shard).Add(1)
		} else {
			s.metrics.TotalWriteCallsErr.Inc()
		}
		return e.loc, e.err
	case <-s.quit:
		return loc, ErrQuitting
	case <-ctx.Done():
		return loc, ctx.Err()
	}
}

// Release gives back the slot to the shard
// From here on the slot can be reused and overwritten
// Release is meant to be called when an entry in the upstream db is removed
// Note that releasing is not safe for obfuscating earlier content, since
// even after reuse, the slot may be used by a very short blob and leaves the
// rest of the old blob bytes untouched
func (s *Store) Release(ctx context.Context, loc Location) error {
	sh := s.shards[loc.Shard]
	// we add the current routine and will be Done in slots.process
	sh.slots.wg.Add(1)
	err := sh.release(ctx, loc.Slot)
	s.metrics.TotalReleaseCalls.Inc()
	if err == nil {
		shard := strconv.Itoa(int(sh.index))
		s.metrics.CurrentShardSize.WithLabelValues(shard).Sub(1)
	} else {
		s.metrics.TotalReleaseCallsErr.Inc()
	}
	return err
}
