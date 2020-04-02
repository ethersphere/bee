// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.package storage

package storage

import (
	"context"
	"errors"

	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	ErrNotFound     = errors.New("storage: not found")
	ErrInvalidChunk = errors.New("storage: invalid chunk")
)

// ChunkValidatorFunc validates Swarm chunk address and chunk data
type ChunkValidatorFunc func(swarm.Address, []byte) bool

type Storer interface {
	Get(ctx context.Context, addr swarm.Address) (data []byte, err error)
	Put(ctx context.Context, addr swarm.Address, data []byte) error
}
