// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package addresses

import (
	"context"

	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
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

func (s *addressesGetterStore) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	ch, err := s.getter.Get(ctx, addr)
	if err != nil {
		return nil, err
	}

	return ch, s.fn(ch.Address())
}
