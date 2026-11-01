// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import "github.com/ethersphere/bee/v2/pkg/feeds"

// Factory is an alias for testing.
type Factory = factory

// WrapLookup exposes wrapLookup for tests.
func (f *factory) WrapLookup(t feeds.Type, lookup feeds.Lookup) feeds.Lookup {
	return f.wrapLookup(t, lookup)
}

// MetricsStruct exposes metrics for tests.
func (f *factory) MetricsStruct() metrics {
	return f.metrics
}
