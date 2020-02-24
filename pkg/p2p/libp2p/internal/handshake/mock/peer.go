// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import "github.com/ethersphere/bee/pkg/swarm"

// todo: implement peer registry mocks, export appropriate interface and move those in libp2p so it can be used in handshake
type PeerFinder struct {
	found bool
}

func (p *PeerFinder) SetFound(found bool) {
	p.found = found
}

func (p *PeerFinder) Exists(overlay swarm.Address) (found bool) {
	return p.found
}
