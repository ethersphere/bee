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

	"github.com/hashicorp/go-multierror"
)

// Recovery allows disaster recovery
type Recovery struct {
	shards []*slots
}

func NewRecovery(dir string, shardCnt int, datasize int) (*Recovery, error) {
	shards := make([]*slots, shardCnt)
	for i := 0; i < shardCnt; i++ {
		file, err := os.OpenFile(path.Join(dir, fmt.Sprintf("shard_%03d", i)), os.O_RDWR|os.O_CREATE, 0666)
		if errors.Is(err, fs.ErrNotExist) {
			break
		}
		if err != nil {
			return nil, err
		}
		fi, err := file.Stat()
		if err != nil {
			return nil, err
		}
		if err = file.Close(); err != nil {
			return nil, err
		}
		size := uint32(fi.Size() / int64(datasize))
		ffile, err := os.OpenFile(path.Join(dir, fmt.Sprintf("free_%03d", i)), os.O_RDWR|os.O_CREATE, 0666)
		if err != nil {
			return nil, err
		}
		sl := newSlots(ffile, nil)
		sl.data = make([]byte, size/8)
		shards[i] = sl
	}
	return &Recovery{shards}, nil
}

// Add marks a location as used (not free)
func (r *Recovery) Add(loc Location) error {
	sh := r.shards[loc.Shard]
	l := len(sh.data)
	if diff := int(loc.Slot/8) - l; diff >= 0 {
		sh.extend(diff + 1)
		for i := 0; i <= diff; i++ {
			sh.data[l+i] = 0x0
		}
	}
	sh.push(loc.Slot)
	return nil
}

// Save saves all free slots files of the recovery (without closing)
func (r *Recovery) Save() (err error) {
	for _, sh := range r.shards {
		for i := range sh.data {
			sh.data[i] ^= 0xff
		}
		err = multierror.Append(err, sh.save())
	}
	return err.(*multierror.Error).ErrorOrNil()
}

// Close closes data and free slots files of the recovery (without saving)
func (r *Recovery) Close() (err error) {
	for _, sh := range r.shards {
		err = multierror.Append(err, sh.file.Close())
	}
	return err.(*multierror.Error).ErrorOrNil()
}
