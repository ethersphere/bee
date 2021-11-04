// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"

	"github.com/ethersphere/bee/pkg/swarm"
)

// Steward represents steward.Interface mock.
type Steward struct {
	addr swarm.Address
}

// Reupload implements steward.Interface Reupload method.
// The given address is recorded.
func (s *Steward) Reupload(_ context.Context, addr swarm.Address) error {
	s.addr = addr
	return nil
}

// IsRetrievable implements steward.Interface IsRetrievable method.
// The method always returns true.
func (s *Steward) IsRetrievable(_ context.Context, _ swarm.Address) (bool, error) {
	return true, nil
}

// LastAddress returns the last address given to the Reupload method call.
func (s *Steward) LastAddress() swarm.Address {
	return s.addr
}
