// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package loadsave provides lightweight persistence abstraction
// for manifest operations.
package loadsave

import (
	"bytes"
	"context"
	"errors"

	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/file/joiner"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline/builder"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var errReadonlyLoadSave = errors.New("readonly manifest loadsaver")

// loadSave is needed for manifest operations and provides
// simple wrapping over load and save operations using file
// package abstractions. use with caution since Loader will
// load all of the subtrie of a given hash in memory.
type loadSave struct {
	getter     storage.Getter
	putter     storage.Putter
	pipelineFn func() pipeline.Interface
	rootCh     swarm.Chunk
}

// New returns a new read-write load-saver.
func New(getter storage.Getter, putter storage.Putter, pipelineFn func() pipeline.Interface) file.LoadSaver {
	return &loadSave{
		getter:     getter,
		putter:     putter,
		pipelineFn: pipelineFn,
	}
}

// NewReadonly returns a new read-only load-saver
// which will error on write.
func NewReadonly(getter storage.Getter) file.LoadSaver {
	return &loadSave{
		getter: getter,
	}
}

// NewReadonlyWithRootCh returns a new read-only load-saver
// which will error on write.
func NewReadonlyWithRootCh(getter storage.Getter, rootCh swarm.Chunk) file.LoadSaver {
	return &loadSave{
		getter: getter,
		rootCh: rootCh,
	}
}

func (ls *loadSave) Load(ctx context.Context, ref []byte) ([]byte, error) {
	var j file.Joiner
	if ls.rootCh == nil || !bytes.Equal(ls.rootCh.Address().Bytes(), ref[:swarm.HashSize]) {
		joiner, _, err := joiner.New(ctx, ls.getter, ls.putter, swarm.NewAddress(ref))
		if err != nil {
			return nil, err
		}
		j = joiner
	} else {
		joiner, _, err := joiner.NewJoiner(ctx, ls.getter, ls.putter, swarm.NewAddress(ref), ls.rootCh)
		if err != nil {
			return nil, err
		}
		j = joiner
	}

	buf := bytes.NewBuffer(nil)
	_, err := file.JoinReadAll(ctx, j, buf)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (ls *loadSave) Save(ctx context.Context, data []byte) ([]byte, error) {
	if ls.pipelineFn == nil {
		return nil, errReadonlyLoadSave
	}

	pipe := ls.pipelineFn()
	address, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(data))
	if err != nil {
		return swarm.ZeroAddress.Bytes(), err
	}

	return address.Bytes(), nil
}
