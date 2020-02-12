// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package discovery

import (
	"github.com/ethersphere/bee/pkg/swarm"

	ma "github.com/multiformats/go-multiaddr"
)

type Driver interface {
	BroadcastPeer(addressee swarm.Address, overlay swarm.Address, addr ma.Multiaddr) error
}
