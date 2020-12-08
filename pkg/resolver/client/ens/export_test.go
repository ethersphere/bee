// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ens

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	goens "github.com/wealdtech/go-ens/v3"
)

const SwarmContentHashPrefix = swarmContentHashPrefix

var ErrNotImplemented = errNotImplemented

// WithConnectFunc will set the Dial function implementaton.
func WithConnectFunc(fn func(endpoint string, contractAddr string) (*ethclient.Client, *goens.Registry, error)) Option {
	return func(c *Client) {
		c.connectFn = fn
	}
}

// WithResolveFunc will set the Resolve function implementation.
func WithResolveFunc(fn func(registry *goens.Registry, addr common.Address, input string) (string, error)) Option {
	return func(c *Client) {
		c.resolveFn = fn
	}
}
