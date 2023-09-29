// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sync"

	"github.com/hashicorp/go-multierror"
)

// Recovery allows disaster recovery.
type Recovery struct {
	mtx        sync.Mutex
	shards     []*slots
	shardFiles []*os.File
	datasize   int
}

var ErrShardNotFound = errors.New("shard not found")

func NewRecovery(dir string, shardCnt int, datasize int) (*Recovery, error) {
	shards := make([]*slots, shardCnt)
	shardFiles := make([]*os.File, shardCnt)

	for i := 0; i < shardCnt; i++ {
		file, err := os.OpenFile(path.Join(dir, fmt.Sprintf("shard_%03d", i)), os.O_RDWR, 0666)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("index %d: %w", i, ErrShardNotFound)
		}
		if err != nil {
			return nil, err
		}
		fi, err := file.Stat()
		if err != nil {
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
		shardFiles[i] = file
	}
	return &Recovery{shards: shards, shardFiles: shardFiles, datasize: datasize}, nil
}

// Add marks a location as used (not free).
func (r *Recovery) Add(loc Location) error {
	r.mtx.Lock()
	defer r.mtx.Unlock()

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

func (r *Recovery) Read(ctx context.Context, loc Location, buf []byte) error {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	_, err := r.shardFiles[loc.Shard].ReadAt(buf, int64(loc.Slot)*int64(r.datasize))
	return err
}

func (r *Recovery) Move(ctx context.Context, from Location, to Location) error {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	chData := make([]byte, from.Length)
	_, err := r.shardFiles[from.Shard].ReadAt(chData, int64(from.Slot)*int64(r.datasize))
	if err != nil {
		return err
	}

	_, err = r.shardFiles[to.Shard].WriteAt(chData, int64(to.Slot)*int64(r.datasize))
	return err
}

func (r *Recovery) TruncateAt(ctx context.Context, shard uint8, slot uint32) error {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	return r.shardFiles[shard].Truncate(int64(slot) * int64(r.datasize))
}

// Save saves all free slots files of the recovery (without closing).
func (r *Recovery) Save() error {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	err := new(multierror.Error)
	for _, sh := range r.shards {
		for i := range sh.data {
			sh.data[i] ^= 0xff
		}
		err = multierror.Append(err, sh.save())
	}
	return err.ErrorOrNil()
}

// Close closes data and free slots files of the recovery (without saving).
func (r *Recovery) Close() error {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	err := new(multierror.Error)
	for idx, sh := range r.shards {
		err = multierror.Append(err, sh.file.Close(), r.shardFiles[idx].Close())
	}
	return err.ErrorOrNil()
}
