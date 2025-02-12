// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"

	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Steward represents steward.Interface mock.
type Steward struct {
	addr swarm.Address
}

// Reupload implements steward.Interface Reupload method.
// The given address is recorded.
func (s *Steward) Reupload(_ context.Context, addr swarm.Address, _ postage.Stamper) error {
	s.addr = addr
	return nil
}

// IsRetrievable implements steward.Interface IsRetrievable method.
// The method always returns true.
func (s *Steward) IsRetrievable(_ context.Context, addr swarm.Address) (bool, error) {
	return addr.Equal(s.addr), nil
}

// LastAddress returns the last address given to the Reupload method call.
func (s *Steward) LastAddress() swarm.Address {
	return s.addr
}
