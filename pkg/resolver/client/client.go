// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"github.com/ethersphere/bee/pkg/resolver"
)

// Interface is a resolver client that can connect/disconnect to an external
// Name Resolution Service via an edpoint.
type Interface interface {
	resolver.Interface
	Connect(endpoint string) error
	IsConnected() bool
	Close()
}
