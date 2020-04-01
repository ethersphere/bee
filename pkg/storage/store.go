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
	ErrNotFound = errors.New("storage: not found")
)

type Storer interface {
	Get(ctx context.Context, addr swarm.Address) (chunk swarm.Chunk, err error)
	Put(ctx context.Context, chunk swarm.Chunk) (err error)
	Has(ctx context.Context, addr swarm.Address) (yes bool, err error)
	Delete(ctx context.Context,addr swarm.Address) (err error)
	Count(ctx context.Context) (count int, err error)
	Iterate(func(ch swarm.Chunk) (stop bool, err error)) (err error)
	Close(ctx context.Context) (err error)
}