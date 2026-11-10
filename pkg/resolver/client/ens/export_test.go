// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ens

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

const SwarmContentHashPrefix = swarmContentHashPrefix

var (
	DNSEncode            = dnsEncode
	ResolveError         = resolveError
	UniversalResolverABI = universalResolverABI
)

// WithConnectFunc will set the Dial function implementation.
func WithConnectFunc(fn func(endpoint string, contractAddr string) (*ethclient.Client, error)) Option {
	return func(c *Client) {
		c.connectFn = fn
	}
}

// WithResolveFunc will set the Resolve function implementation.
func WithResolveFunc(fn func(ethCl *ethclient.Client, addr common.Address, input string) (string, error)) Option {
	return func(c *Client) {
		c.resolveFn = fn
	}
}
