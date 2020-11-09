// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package addresses

import (
	"context"
	"errors"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	// ErrStopIterator is returned iterator function marks that iteration should
	// be stopped.
	ErrStopIterator = errors.New("stop iterator")
)

type addressesGetterStore struct {
	getter storage.Getter
	fn     swarm.AddressIterFunc
}

// NewGetter creates a new proxy storage.Getter which calls provided function
// for each chunk address processed.
func NewGetter(getter storage.Getter, fn swarm.AddressIterFunc) storage.Getter {
	return &addressesGetterStore{getter, fn}
}

func (s *addressesGetterStore) Get(ctx context.Context, mode storage.ModeGet, addr swarm.Address) (ch swarm.Chunk, err error) {
	ch, err = s.getter.Get(ctx, mode, addr)
	if err != nil {
		return
	}

	stop := s.fn(ch.Address())
	if stop {
		return ch, ErrStopIterator
	}

	return
}
