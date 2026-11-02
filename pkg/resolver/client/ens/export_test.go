// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ens

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	goens "github.com/wealdtech/go-ens/v3"
)

const SwarmContentHashPrefix = swarmContentHashPrefix

// WithConnectFunc will set the Dial function implementation.
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

// WithResolveDirectFunc will set the direct (resolver-profile) Resolve
// function implementation.
func WithResolveDirectFunc(fn func(ethCl *ethclient.Client, addr common.Address, input string) (string, error)) Option {
	return func(c *Client) {
		c.resolveDirectFn = fn
	}
}

// ContractCaller is the subset of an Ethereum client used by the EIP-165 probe.
type ContractCaller = contractCaller

// SupportsContenthash exposes the EIP-165 probe for testing.
func SupportsContenthash(ctx context.Context, caller ContractCaller, addr common.Address) (bool, error) {
	return supportsContenthash(ctx, caller, addr)
}
