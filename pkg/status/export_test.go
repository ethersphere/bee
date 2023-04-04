// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status

import (
	"context"

	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	ProtocolName    = protocolName
	ProtocolVersion = protocolVersion
	StreamName      = streamName
)

// SendGetSnapshotMsg sends a Get Snapshot message to the peer.
func (s *Service) SendGetSnapshotMsg(ctx context.Context, address swarm.Address) error {
	_, err := s.requestStatusSnapshot(ctx, address)
	return err
}
