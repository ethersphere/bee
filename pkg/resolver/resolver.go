// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package resolver

import (
	"github.com/ethersphere/bee/pkg/swarm"
)

// Address is the swarm bzz address.
type Address = swarm.Address

// Interface can resolve an URL into an associated Ethereum address.
type Interface interface {
	Resolve(url string) (Address, error)
}
