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

	ErrTooLong = errors.New("data too long")
)

// models the sharded chunkdb
type Shards struct {
	writes chan write // shared write operations channel
	pool   *sync.Pool // pool to save on allocating for operation
	shards []*shard
	quit   chan struct{}
}

// New constructs a new sharded chunk db
func New(basedir string, shardCnt int, limit uint32) (*Shards, error) {
	pool := &sync.Pool{New: func() interface{} {
		return make(chan entry)
	}}
	sh := &Shards{
		pool:   pool,
		writes: make(chan write),
		shards: make([]*shard, shardCnt),
		quit:   make(chan struct{}),
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
		reads:  make(chan read),
		errc:   make(chan error),
		writes: s.writes,
		index:  index,
		file:   file,
		slots:  sl,
		quit:   s.quit,
	}
	terminated := make(chan struct{})
	go func() {
		sh.process()
		close(terminated)
	}()
	go sl.process(terminated)
	return sh, nil
}

func (s *Shards) Read(ctx context.Context, loc Location, buf []byte) (err error) {
	sh := s.shards[loc.Shard]
	select {
	case sh.reads <- read{buf[:loc.Length], loc.Slot, 0}:
	case <-ctx.Done():
		return ctx.Err()
	}
	return <-sh.errc
}

func (s *Shards) Write(ctx context.Context, data []byte) (loc Location, err error) {
	if len(data) > int(DataSize) {
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

func (s *Shards) Release(ctx context.Context, loc Location) {
	sh := s.shards[loc.Shard]
	sh.release(loc.Slot)
}
