// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"github.com/ethersphere/bee/pkg/retrieval"
	"github.com/ethersphere/bee/pkg/swarm"
)

type mock struct {
	checkAvailableChunkFunc func(ctx context.Context, addr swarm.Address) (err error)
}

func New(checkAvailableChunk func(ctx context.Context, addr swarm.Address) error) retrieval.Verifier {
	return &mock{checkAvailableChunkFunc: checkAvailableChunk}
}

func (s *mock) CheckAvailableChunk(ctx context.Context, addr swarm.Address) (err error) {
	if s.checkAvailableChunkFunc != nil {
		return s.checkAvailableChunkFunc(ctx, addr)
	}

	return nil
}
