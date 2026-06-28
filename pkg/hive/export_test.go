// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	"context"
	"time"

	"github.com/ethersphere/bee/v2/pkg/hive/pb"
)

var (
	MaxBatchSize = maxBatchSize
	LimitBurst   = limitBurst
)

func (s *Service) SetTimeFunc(f func() time.Time) {
	s.now = f
}

// SetCoalesceJitterForTest fixes coalesce deadline jitter for deterministic tests.
func (s *Service) SetCoalesceJitterForTest(d time.Duration) {
	s.gossipBuf.jitter = d
}

// FlushDueGossipForTest flushes coalesced gossip entries whose deadline has passed.
func (s *Service) FlushDueGossipForTest() {
	s.flushGossipEntries(s.gossipBuf.takeDue(s.now()), coalesceFlushReasonTimer)
}

// CheckAndAddPeers exposes the internal ingestion path for tests,
// bypassing the stream and rate limiter.
func (s *Service) CheckAndAddPeers(peers pb.Peers) {
	s.checkAndAddPeers(context.Background(), peers)
}
