// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	"context"
	"time"

	"github.com/ethersphere/bee/v2/pkg/hive/pb"
)

var MaxBatchSize = maxBatchSize
var LimitBurst = limitBurst

func (s *Service) SetTimeFunc(f func() time.Time) {
	s.now = f
}

// CheckAndAddPeers exposes the internal ingestion path for tests,
// bypassing the stream and rate limiter.
func (s *Service) CheckAndAddPeers(peers pb.Peers) {
	s.checkAndAddPeers(context.Background(), peers)
}
