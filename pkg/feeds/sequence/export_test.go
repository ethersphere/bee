// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sequence

import (
	"github.com/ethersphere/bee/pkg/feeds"
)

func Index(i feeds.Index) int {
	return int(i.(*index).index)
}
