// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import "github.com/ethersphere/bee/v2/pkg/feeds"

func WrapLookup(f feeds.Factory, t feeds.Type, lookup feeds.Lookup) feeds.Lookup {
	return f.(*factory).wrapLookup(t, lookup)
}
