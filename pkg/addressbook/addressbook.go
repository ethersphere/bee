// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package addressbook

import (
	"github.com/ethersphere/bee/pkg/swarm"

	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
)

type GetPutter interface {
	Getter
	Putter
}

type Getter interface {
	Get(overlay swarm.Address) (underlay libp2ppeer.ID, exists bool)
}

type Putter interface {
	Put(underlay libp2ppeer.ID, overlay swarm.Address) (exists bool)
}
