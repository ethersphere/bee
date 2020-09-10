// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"github.com/ethersphere/bee/pkg/resolver"
)

// Interface is a resolver client that can connect/disconnect to an external
// Name Resolution Service via an endpoint.
type Interface interface {
	resolver.Interface
	Endpoint() string
	IsConnected() bool
}
