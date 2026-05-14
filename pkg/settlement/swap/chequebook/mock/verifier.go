// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Verifier is a test double for chequebook.Verifier. It records call count
// and delegates to Behavior (if set) for the verification result.
type Verifier struct {
	Behavior func(chequebook, peerEth common.Address, overlay swarm.Address, verified bool) error
	Calls    int
}

func (v *Verifier) Verify(_ context.Context, cb, peerEth common.Address, overlay swarm.Address, verified bool) error {
	v.Calls++
	if v.Behavior == nil {
		return nil
	}
	return v.Behavior(cb, peerEth, overlay, verified)
}

// Storer is a test double that satisfies both the hive and libp2p
// ChequebookStorer interfaces. It records overlay → chequebook puts and
// invokes the writeAddressbook callback when supplied. Remove is a no-op.
type Storer struct {
	Puts        map[string]common.Address
	AbCallbacks int
}

func (s *Storer) Put(peer swarm.Address, cb common.Address, _ int64, _ bzz.TimestampSource, write func() error) error {
	if s.Puts == nil {
		s.Puts = make(map[string]common.Address)
	}
	if (cb != common.Address{}) {
		s.Puts[peer.String()] = cb
	}
	if write != nil {
		s.AbCallbacks++
		if err := write(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Storer) Remove(_ swarm.Address) {}
