//go:build js
// +build js

package sharky

import (
	"context"
	"io/fs"
	"sync"
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
	}
	for i := range store.shards {
		s, err := store.create(uint8(i), maxDataSize, basedir)
		if err != nil {
			return nil, err
		}
		store.shards[i] = s
	}

	return store, nil
}

// Read reads the content of the blob found at location into the byte buffer given
// The location is assumed to be obtained by an earlier Write call storing the blob
func (s *Store) Read(ctx context.Context, loc Location, buf []byte) (err error) {
	sh := s.shards[loc.Shard]
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-sh.quit:
		return ErrQuitting
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
	err := sh.release(ctx, loc.Slot)

	return err
}

// Write stores a new blob and returns its location to be used as a reference
// It can be given to a Read call to return the stored blob.
func (s *Store) Write(ctx context.Context, data []byte) (loc Location, err error) {
	if len(data) > s.maxDataSize {
		return loc, ErrTooLong
	}
	s.wg.Add(1)
	defer s.wg.Done()

	c := make(chan entry, 1) // buffer the channel to avoid blocking in shard.process on quit or context done

	select {
	case <-s.quit:
		return loc, ErrQuitting
	case <-ctx.Done():
		return loc, ctx.Err()
	}

	select {
	case e := <-c:

		return e.loc, e.err
	case <-s.quit:
		return loc, ErrQuitting
	case <-ctx.Done():
		return loc, ctx.Err()
	}
}
