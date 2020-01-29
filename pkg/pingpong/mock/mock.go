// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"time"

	"github.com/ethersphere/bee/pkg/swarm"
)

type Service struct {
	pingFunc func(ctx context.Context, address swarm.Address, msgs ...string) (rtt time.Duration, err error)
}

func New(pingFunc func(ctx context.Context, address swarm.Address, msgs ...string) (rtt time.Duration, err error)) *Service {
	return &Service{pingFunc: pingFunc}
}

func (s *Service) Ping(ctx context.Context, address swarm.Address, msgs ...string) (rtt time.Duration, err error) {
	return s.pingFunc(ctx, address, msgs...)
}
