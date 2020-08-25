// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ens

import (
	"errors"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/ethersphere/bee/pkg/resolver/client"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Address is the swarm bzz address.
type Address = swarm.Address

// Make sure Client implements the resolver.Client interface.
var _ client.Interface = (*Client)(nil)

type dialType func(string) (*ethclient.Client, error)
type resolveType func(bind.ContractBackend, string) (string, error)

// Client is a name resolution client that can connect to ENS via an
// Ethereum endpoint.
type Client struct {
	Endpoint  string
	ethCl     *ethclient.Client
	dialFn    dialType
	resolveFn resolveType
}

// Option is a function that applies an option to a Client.
type Option func(*Client)

// NewClient will return a new Client.
func NewClient(opts ...Option) *Client {
	c := &Client{
		dialFn:    wrapDial,
		resolveFn: wrapResolve,
	}

	// Apply all options to the Client.
	for _, o := range opts {
		o(c)
	}

	return c
}

// Connect implements the resolver.Client interface.
func (c *Client) Connect(ep string) error {
	if c.dialFn == nil {
		return errors.New("no dial function implementation")
	}

	ethCl, err := c.dialFn(ep)
	if err != nil {
		return err
	}

	c.Endpoint = ep
	c.ethCl = ethCl
	return nil
}

// IsConnected returns true if there is an active RPC connection with an
// Ethereum node at the configured endpoint.
func (c *Client) IsConnected() bool {
	return c.ethCl != nil
}

// Resolve implements the resolver.Client interface.
func (c *Client) Resolve(name string) (Address, error) {
	if c.resolveFn == nil {
		return swarm.ZeroAddress, errors.New("no resolve function implementation")
	}

	// Retrieve the content hash from ENS.
	hash, err := c.resolveFn(c.ethCl, name)
	if err != nil {
		return swarm.ZeroAddress, err
	}

	// Ensure that the content hash string is in a valid format, eg.
	// "/swarm/<address>".
	if !strings.HasPrefix(hash, "/swarm/") {
		return swarm.ZeroAddress, errors.New("ENS contenthash invalid")
	}

	// Trim the prefix and try to parse the result as a bzz address.
	return swarm.ParseHexAddress(strings.TrimPrefix(hash, "/swarm/"))
}

// Close closes the RPC connection with the client, terminating all unfinished
// requests.
func (c *Client) Close() {
	if c.ethCl != nil {
		c.ethCl.Close() // TODO: consider mocking out the eth client.
	}
	c.ethCl = nil
}
