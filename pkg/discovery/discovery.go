// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package discovery

import (
	"context"

	"github.com/ethersphere/bee/pkg/swarm"
	ma "github.com/multiformats/go-multiaddr"
)

type BroadcastRecord struct {
	Overlay swarm.Address
	Addr    ma.Multiaddr
}

type Driver interface {
	BroadcastPeers(ctx context.Context, addressee swarm.Address, peers ...BroadcastRecord) error
}
