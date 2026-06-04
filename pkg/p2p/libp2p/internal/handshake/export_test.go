// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	"context"
	"time"

	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/handshake/pb"
	ma "github.com/multiformats/go-multiaddr"
)

const MaxCachedAddresses = maxCachedAddresses

func (s *Service) SetTime(f func() time.Time) {
	s.now = f
}

func (s *Service) ParseCheckAck(ctx context.Context, ack *pb.Ack) (*bzz.Address, error) {
	return s.parseCheckAck(ctx, ack)
}

func (s *Service) SignedAddress(underlays []ma.Multiaddr) (*bzz.Address, error) {
	return s.signedAddress(underlays)
}

func (s *Service) AddressCacheLen() int {
	s.addrCacheMu.Lock()
	defer s.addrCacheMu.Unlock()
	return s.addrCacheLRU.Len()
}
