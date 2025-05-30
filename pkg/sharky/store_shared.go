// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/hashicorp/go-multierror"
)

var (
	// ErrTooLong returned by Write if the blob length exceeds the max blobsize.
	ErrTooLong = errors.New("data too long")
	// ErrQuitting returned by Write when the store is Closed before the write completes.
	ErrQuitting = errors.New("quitting")
)

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
func (s *Store) create(index uint8, maxDataSize int, basedir fs.FS) (*shard, error) {
	file, err := basedir.Open(fmt.Sprintf("shard_%03d", index))
	if err != nil {
		return nil, err
	}
	ffile, err := basedir.Open(fmt.Sprintf("free_%03d", index))
	if err != nil {
		return nil, err
	}
	sl := newSlots(ffile.(sharkyFile), s.wg)
	err = sl.load()
	if err != nil {
		return nil, err
	}
	sh := &shard{
		reads:       make(chan read),
		errc:        make(chan error),
		writes:      s.writes,
		index:       index,
		maxDataSize: maxDataSize,
		file:        file.(sharkyFile),
		slots:       sl,
		quit:        s.quit,
	}
	terminated := make(chan struct{})
	sh.slots.wg.Add(1)
	go func() {
		defer sh.slots.wg.Done()
		sh.process()
		close(terminated)
	}()
	sh.slots.wg.Add(1)
	go func() {
		defer sh.slots.wg.Done()
		sl.process(terminated)
	}()
	return sh, nil
}
