// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pricer

import (
	"github.com/ethersphere/bee/pkg/swarm"
)

func (s *Pricer) PeerPricePO(peer swarm.Address, PO uint8) (uint64, error) {
	price, err := s.peerPricePO(peer, PO)
	return price, err
}
