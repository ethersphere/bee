// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
)

// TestSetIsWarmingUpNilReceiver is a regression test for NIL-01. When the API
// is disabled (empty --api-addr) the apiService is nil, and the warmup
// completion goroutine calls SetIsWarmingUp on it. Like its sibling setters,
// SetIsWarmingUp must be a no-op on a nil receiver rather than panicking.
func TestSetIsWarmingUpNilReceiver(t *testing.T) {
	t.Parallel()

	var s *api.Service // nil, as when the API is disabled

	// Must not panic.
	s.SetIsWarmingUp(false)
	s.SetIsWarmingUp(true)
}
