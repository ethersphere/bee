// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package p2p

import "errors"

type Peer struct {
	Address string
}

var ErrPeerNotFound = errors.New("peer not found")
