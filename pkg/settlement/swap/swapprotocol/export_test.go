// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swapprotocol

import (
	"context"
	"github.com/ethersphere/bee/pkg/p2p"
)

func (s *Service) Init(ctx context.Context, p p2p.Peer) error {
	return s.init(ctx, p)
}
