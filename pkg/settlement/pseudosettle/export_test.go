// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pseudosettle

import (
	"context"
	"time"

	"github.com/ethersphere/bee/pkg/p2p"
)

const (
	AllowanceFieldName = allowanceFieldName
	TimestampFieldName = timestampFieldName
)

func (s *Service) SetTimeNow(f func() time.Time) {
	s.timeNow = f
}

func (s *Service) SetTime(k int64) {
	s.SetTimeNow(func() time.Time {
		return time.Unix(k, 0)
	})
}

func (s *Service) Init(ctx context.Context, peer p2p.Peer) error {
	return s.init(ctx, peer)
}

func (s *Service) Terminate(peer p2p.Peer) error {
	return s.terminate(peer)
}
