// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	"context"
	"time"

	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/handshake/pb"
)

func (s *Service) SetTime(f func() time.Time) {
	s.now = f
}

func (s *Service) ParseCheckAck(ctx context.Context, ack *pb.Ack) (*bzz.Address, error) {
	return s.parseCheckAck(ctx, ack)
}
