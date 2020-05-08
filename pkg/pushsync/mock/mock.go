// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"

	"github.com/ethersphere/bee/pkg/pushsync/pb"
	"github.com/ethersphere/bee/pkg/swarm"
)

type PushSync struct {
	SendChunk func(ctx context.Context, peer swarm.Address, chunk swarm.Chunk) (*pb.Receipt, error)
}

func New(sendChunk func(ctx context.Context, peer swarm.Address, chunk swarm.Chunk) (*pb.Receipt, error)) *PushSync {
	return &PushSync{SendChunk: sendChunk}
}

func (s *PushSync) SendChunkAndReceiveReceipt(ctx context.Context, address swarm.Address, chunk swarm.Chunk) (*pb.Receipt, error) {
	return s.SendChunk(ctx, address, chunk)
}
