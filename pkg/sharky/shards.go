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
	"strings"
	"sync"

	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	DataSize int64 = swarm.ChunkWithSpanSize

	ErrTooLong         = errors.New("data too long")
	ErrCapacityReached = errors.New("capacity reached")
)

// models the sharded chunkdb
type Shards struct {
	writeOps chan *operation // shared write operations channel
	pool     *sync.Pool      // pool to save on allocating for operation
	shards   []*shard
	quit     chan struct{}
}

// New constructs a new sharded chunk db
func New(basedir string, shardCnt int, limit uint32) (*Shards, error) {
	pool := &sync.Pool{New: func() interface{} {
		return newOp()
	}}
	sh := &Shards{
		pool:     pool,
		writeOps: make(chan *operation),
		shards:   make([]*shard, shardCnt),
		quit:     make(chan struct{}),
	}
	for i := range sh.shards {
		s, err := sh.create(uint8(i), limit, basedir)
		if err != nil {
			return nil, err
		}
		sh.shards[i] = s
	}
	return sh, nil
}

// Close closes each shard
func (s *Shards) Close() error {
	close(s.quit)
	errs := []string{}
	errc := make(chan error)
	for _, sh := range s.shards {
		sh := sh
		go func() {
			errc <- sh.close()
		}()
	}
	for range s.shards {
		if err := <-errc; err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("closing shards: %s", strings.Join(errs, ", "))
	}
	return nil
}

// create creates a new shard with index, max capacity limit, file within base directory
func (s *Shards) create(index uint8, limit uint32, basedir string) (*shard, error) {
	file, err := os.OpenFile(path.Join(basedir, fmt.Sprintf("shard_%03d", index)), os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	fi, err := file.Stat()
	if err != nil {
		return nil, err
	}
	size := uint32(fi.Size() / DataSize)
	ffile, err := os.OpenFile(path.Join(basedir, fmt.Sprintf("free_%03d", index)), os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	sl := newSlots(size, ffile, limit)
	err = sl.load()
	if err != nil {
		return nil, err
	}
	sh := &shard{
		readOps:  make(chan *operation),
		writeOps: s.writeOps,
		index:    index,
		file:     file,
		slots:    sl,
		quit:     s.quit,
	}
	go sh.process()
	go sl.process(s.quit)
	return sh, nil
}

func (s *Shards) Read(ctx context.Context, loc Location) (data []byte, err error) {
	op, f := s.newReadOp(loc)
	defer f()

	sh := s.shards[loc.Shard]
	select {
	case sh.readOps <- op:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case err = <-op.err:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return op.buffer[:op.location.Length], err
}

func (s *Shards) Write(ctx context.Context, data []byte) (loc Location, err error) {
	if len(data) > int(DataSize) {
		return loc, ErrTooLong
	}
	op, f := s.newWriteOp(data)
	defer f()

	select {
	case s.writeOps <- op:
	case <-ctx.Done():
		return loc, ctx.Err()
	}

	select {
	case err = <-op.err:
	case <-ctx.Done():
		return loc, ctx.Err()
	}
	return op.location, err
}

func (s *Shards) Release(ctx context.Context, loc Location) {
	sh := s.shards[loc.Shard]
	sh.release(loc.Slot)
}

func newOp() *operation {
	return &operation{
		location: Location{},
		err:      make(chan error),
		buffer:   make([]byte, DataSize),
	}
}

func (s *Shards) newReadOp(loc Location) (*operation, func()) {
	op := s.pool.Get().(*operation)
	f := func() { s.pool.Put(op) }
	op.location = loc
	return op, f
}

func (s *Shards) newWriteOp(data []byte) (*operation, func()) {
	op := s.pool.Get().(*operation)
	f := func() { s.pool.Put(op) }
	op.data = data
	return op, f
}
