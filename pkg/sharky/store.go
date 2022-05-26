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
	maxDataSize int           // max length of blobs
	writes      chan write    // shared write operations channel
	shards      []*shard      // shards
	quit        chan struct{} // quit channel
	metrics     metrics
}

// New constructs a sharded blobstore
// arguments:
// - base directory string
// - shard count - positive integer < 256 - cannot be zero or expect panic
// - shard size - positive integer (practically multiple of 8 on disk) chunk per shard
// - maxDataSize - positive integer representing the maximum blob size to be stored
func New(basedir fs.FS, shardCnt, shardSize, maxDataSize int) (*Store, error) {
	store := &Store{
		maxDataSize: maxDataSize,
		writes:      make(chan write),
		shards:      make([]*shard, shardCnt),
		quit:        make(chan struct{}),
		metrics:     newMetrics(),
	}
	for i := range store.shards {
		s, err := store.create(uint8(i), shardSize, maxDataSize, basedir)
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
func (s *Store) create(index uint8, shardSize, maxDataSize int, basedir fs.FS) (*shard, error) {
	file, err := basedir.Open(fmt.Sprintf("shard_%03d", index))
	if err != nil {
		return nil, err
	}
	ffile, err := basedir.Open(fmt.Sprintf("free_%03d", index))
	if err != nil {
		return nil, err
	}
	sl := newSlots(ffile.(sharkyFile), shardSize)
	err = sl.load()
	if err != nil {
		return nil, err
	}
	sh := &shard{
		reads:       make(chan read),
		errc:        make(chan error, 1),
		writes:      s.writes,
		index:       index,
		maxDataSize: maxDataSize,
		file:        file.(sharkyFile),
		slots:       sl,
		quit:        s.quit,
	}
	go sh.process()
	go sl.process(s.quit)
	return sh, nil
}

// Read reads the content of the blob found at location into the byte buffer given
// The location is assumed to be obtained by an earlier Write call storing the blob
func (s *Store) Read(ctx context.Context, loc Location, buf []byte) (err error) {
	sh := s.shards[loc.Shard]
	select {
	case sh.reads <- read{buf: buf[:loc.Length], slot: loc.Slot}:
		s.metrics.TotalReadCalls.Inc()
	case <-ctx.Done():
		return ctx.Err()
	case <-sh.quit:
		return ErrQuitting
	}

	// if one select case is context cancellation, then in order to avoid deadlock
	// the result of the operation must be drained from rerrc, allowing the
	// shard to be able to handle new operations (#2932).
	select {
	case err = <-sh.errc:
		if err != nil {
			s.metrics.TotalReadCallsErr.Inc()
		}
		return err
	case <-ctx.Done():
		<-sh.errc
		return ctx.Err()
	case <-s.quit:
		// we need to make sure that the forever loop in shard.go can
		// always return due to shutdown in case this goroutine goes away.
		return ErrQuitting
	}
}

// Write stores a new blob and returns its location to be used as a reference
// It can be given to a Read call to return the stored blob.
func (s *Store) Write(ctx context.Context, data []byte) (loc Location, err error) {
	if len(data) > s.maxDataSize {
		return loc, ErrTooLong
	}
	op := write{data, &loc, make(chan error)}
	select {
	case s.writes <- op:
		s.metrics.TotalWriteCalls.Inc()
	case <-s.quit:
		return loc, ErrQuitting
	case <-ctx.Done():
		return loc, ctx.Err()
	}

	// select {
	// case err := <-op.errc:
	err = <-op.errc
	if err == nil {
		loc = *(op.loc)
		shard := strconv.Itoa(int(loc.Shard))
		s.metrics.CurrentShardSize.WithLabelValues(shard).Inc()
		s.metrics.ShardFragmentation.WithLabelValues(shard).Add(float64(s.maxDataSize - int(loc.Length)))
	} else {
		s.metrics.TotalWriteCallsErr.Inc()
	}
	return loc, err
	// case <-s.quit:
	// 	return loc, ErrQuitting
	// case <-ctx.Done():
	// 	return loc, ctx.Err()
	// }
}

// Release gives back the slot to the shard
// From here on the slot can be reused and overwritten
// Release is meant to be called when an entry in the upstream db is removed
// Note that releasing is not safe for obfuscating earlier content, since
// even after reuse, the slot may be used by a very short blob and leaves the
// rest of the old blob bytes untouched
func (s *Store) Release(ctx context.Context, loc Location) error {
	sh := s.shards[loc.Shard]
	err := sh.release(ctx, loc.Slot)
	s.metrics.TotalReleaseCalls.Inc()
	if err == nil {
		shard := strconv.Itoa(int(sh.index))
		s.metrics.CurrentShardSize.WithLabelValues(shard).Dec()
		s.metrics.ShardFragmentation.WithLabelValues(shard).Sub(float64(s.maxDataSize - int(loc.Length)))
	} else {
		s.metrics.TotalReleaseCallsErr.Inc()
	}
	return err
}
