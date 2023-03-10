// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
)

// Recovery allows disaster recovery.
type Recovery struct {
	slots []*slots
}

var ErrShardNotFound = errors.New("shard not found")

func NewRecovery(dir string, shardCnt int, datasize int) (*Recovery, error) {
	slots := make([]*slots, shardCnt)
	for i := 0; i < shardCnt; i++ {
		file, err := os.OpenFile(path.Join(dir, fmt.Sprintf("shard_%03d", i)), os.O_RDONLY, 0666)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("index %d: %w", i, ErrShardNotFound)
			}
			return nil, err
		}
		if err = file.Close(); err != nil {
			return nil, err
		}
		ffile, err := os.OpenFile(path.Join(dir, fmt.Sprintf("free_%03d", i)), os.O_RDWR|os.O_CREATE, 0666)
		if err != nil {
			return nil, err
		}
		slots[i] = newSlots(ffile)
	}
	return &Recovery{slots}, nil
}

// Use marks a location as used (not free).
func (r *Recovery) Use(loc Location) {
	r.slots[loc.Shard].Use(loc.Slot)
}

// Close saves all free slots files of the recovery and closes data and free slots files of the recovery.
func (r *Recovery) Close() (err error) {
	for _, sl := range r.slots {
		err = errors.Join(sl.Close())
	}
	return
}
