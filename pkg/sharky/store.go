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
	"time"
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
	maxDataSize     int            // max length of blobs
	shards          []*shard       // shards
	wg              sync.WaitGroup // count started operations
	quit            chan struct{}  // quit channel
	metrics         metrics
	availableShards chan availableShard
}

type availableShard struct {
	shard uint8
	slot  uint32
}

// New constructs a sharded blobstore
// arguments:
// - base directory string
// - shard count - positive integer < 256 - cannot be zero or expect panic
// - shard size - positive integer multiple of 8 - for others expect undefined behaviour
// - maxDataSize - positive integer representing the maximum blob size to be stored
func New(basedir fs.FS, shardCnt int, maxDataSize int) (*Store, error) {
	store := &Store{
		maxDataSize:     maxDataSize,
		shards:          make([]*shard, shardCnt),
		quit:            make(chan struct{}),
		metrics:         newMetrics(),
		availableShards: make(chan availableShard),
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

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(time.Second * 5):
		return errors.New("did not close on time")
	}
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
	sl := newSlots(ffile.(sharkyFile))
	err = sl.load()
	if err != nil {
		return nil, err
	}
	sh := &shard{
		available:   s.availableShards,
		index:       index,
		maxDataSize: maxDataSize,
		file:        file.(sharkyFile),
		slots:       sl,
		quit:        s.quit,
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		sh.process()
	}()
	return sh, nil
}

// Read reads the content of the blob found at location into the byte buffer given
// The location is assumed to be obtained by an earlier Write call storing the blob
func (s *Store) Read(loc Location, buf []byte) (err error) {
	sh := s.shards[loc.Shard]
	return sh.read(buf[:loc.Length], loc.Slot)
}

// Write stores a new blob and returns its location to be used as a reference
// It can be given to a Read call to return the stored blob.
func (s *Store) Write(ctx context.Context, data []byte) (loc Location, err error) {
	if len(data) > s.maxDataSize {
		return loc, ErrTooLong
	}

	s.wg.Add(1)
	defer s.wg.Done()

	select {
	case write := <-s.availableShards:
		loc, err := s.shards[write.shard].write(data, write.slot)
		if err != nil {
			s.metrics.TotalWriteCallsErr.Inc()
			return loc, err
		}
		shard := strconv.Itoa(int(loc.Shard))
		s.metrics.CurrentShardSize.WithLabelValues(shard).Inc()
		s.metrics.ShardFragmentation.WithLabelValues(shard).Add(float64(s.maxDataSize - int(loc.Length)))
		s.metrics.LastAllocatedShardSlot.WithLabelValues(shard).Set(float64(loc.Slot))
		return loc, nil
	case <-ctx.Done():
		s.metrics.TotalWriteCallsErr.Inc()
		return loc, ctx.Err()
	}
}

// Release gives back the slot to the shard
// From here on the slot can be reused and overwritten
// Release is meant to be called when an entry in the upstream db is removed
// Note that releasing is not safe for obfuscating earlier content, since
// even after reuse, the slot may be used by a very short blob and leaves the
// rest of the old blob bytes untouched
func (s *Store) Release(loc Location) {
	s.metrics.TotalReleaseCalls.Inc()
	sh := s.shards[loc.Shard]
	sh.release(loc.Slot)
	shard := strconv.Itoa(int(sh.index))
	s.metrics.CurrentShardSize.WithLabelValues(shard).Dec()
	s.metrics.ShardFragmentation.WithLabelValues(shard).Sub(float64(s.maxDataSize - int(loc.Length)))
	s.metrics.LastReleasedShardSlot.WithLabelValues(shard).Set(float64(loc.Slot))
}
