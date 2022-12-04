// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky

import (
	"fmt"
	"os"
	"path"

	"github.com/hashicorp/go-multierror"
)

// Recovery allows disaster recovery.
type Compaction struct {
	s         *Store
	shardCnt  int
	dataSize  int
	shardPool map[uint8]*shard
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

		ffile, err := os.OpenFile(path.Join(dir, fmt.Sprintf("free_%03d", i)), os.O_RDONLY, 0666)
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

	c := &Compaction{s, shardCnt, datasize, make(map[uint8]*shard)}

	for i, shard := range s.shards {
		c.shardPool[uint8(i)] = shard
	}

	return c, nil

}

func (c *Compaction) Write(oldLoc Location, data []byte) (bool, Location, error) {

	if shard, ok := c.shardPool[oldLoc.Shard]; !ok {
		return false, Location{}, nil
	} else {
		fragmented, freeSlot := shard.slots.Fragmented()

		if !fragmented {
			delete(c.shardPool, oldLoc.Shard)
			return false, Location{}, nil
		}

		if oldLoc.Slot > freeSlot {
			newLoc, err := shard.write(data, freeSlot)
			if err != nil {
				return false, Location{}, err
			}

			shard.slots.Use(freeSlot)
			shard.slots.Free(oldLoc.Slot)

			return true, newLoc, nil
		}

		return false, Location{}, nil
	}
}

func (r *Compaction) Close() error {
	err := new(multierror.Error)

	for _, shard := range r.s.shards {
		freeSlot := shard.slots.Next()
		err = multierror.Append(err, shard.file.Truncate(int64(freeSlot*uint32(r.dataSize))))
	}

	err = multierror.Append(err, r.s.Close())
	return err.ErrorOrNil()
}
