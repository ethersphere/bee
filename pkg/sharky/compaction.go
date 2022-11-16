// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/hashicorp/go-multierror"
)

// Recovery allows disaster recovery.
type Compaction struct {
	s        *Store
	shardCnt int
	dataSize int
}

func NewCompaction(dir string, shardCnt int, datasize int) (*Compaction, error) {
	for i := 0; i < shardCnt; i++ {
		file, err := os.OpenFile(path.Join(dir, fmt.Sprintf("shard_%03d", i)), os.O_RDONLY, 0666)
		if err != nil {
			return nil, err
		}
		if err = file.Close(); err != nil {
			return nil, err
		}

		ffile, err := os.Create(path.Join(dir, fmt.Sprintf("free_%03d", i)))
		if err != nil {
			return nil, err
		}
		if err = ffile.Close(); err != nil {
			return nil, err
		}
	}

	s, err := New(os.DirFS(dir), shardCnt, datasize)
	if err != nil {
		return nil, err
	}

	return &Compaction{s, shardCnt, datasize}, nil
}

// Use marks a location as used (not free).
func (c *Compaction) Write(ctx context.Context, data []byte) (Location, error) {
	return c.s.Write(ctx, data)
}

// Close closes data and free slots files of the recovery (without saving).
func (r *Compaction) Close() error {
	err := new(multierror.Error)

	for _, shard := range r.s.shards {
		freeSlot := shard.slots.Next()
		err = multierror.Append(err, shard.file.Truncate(int64(freeSlot*uint32(r.dataSize))))
	}

	err = multierror.Append(err, r.s.Close())
	return err.ErrorOrNil()
}
