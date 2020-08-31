// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import "github.com/ethersphere/bee/pkg/storage"

type peerBlocker struct {
	store storage.StateStorer
}
