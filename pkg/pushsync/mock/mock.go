// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"

	"github.com/ethersphere/bee/pkg/pushsync"
	"github.com/ethersphere/bee/pkg/swarm"
)

type PushSync struct {
	sendChunk func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error)
}

func New(sendChunk func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error)) *PushSync {
	return &PushSync{sendChunk: sendChunk}
}

func (s *PushSync) PushChunkToClosest(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
	return s.sendChunk(ctx, chunk)
}
