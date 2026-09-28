// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redistribution

import "math/big"

// CanOverrideClaim exposes the unexported canOverrideClaim method for tests.
func CanOverrideClaim(c Contract, opts *ClaimOpts, gasFeeCap *big.Int) bool {
	return c.(*contract).canOverrideClaim(opts, gasFeeCap)
}
