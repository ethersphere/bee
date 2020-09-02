// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ens

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
	goens "github.com/wealdtech/go-ens/v3"

	"github.com/ethersphere/bee/pkg/resolver/client"
	"github.com/ethersphere/bee/pkg/swarm"
)

const swarmContentHashPrefix = "/swarm/"

// Address is the swarm bzz address.
type Address = swarm.Address

// Make sure Client implements the resolver.Client interface.
var _ client.Interface = (*Client)(nil)

var (
	// ErrFailedToConnect denotes that the resolver failed to connect to the
	// provided endpoint.
	ErrFailedToConnect = errors.New("failed to connect")
	// ErrResolveFailed denotes that a name could not be resolved.
	ErrResolveFailed = errors.New("resolve failed")
	// ErrInvalidContentHash denotes that the value of the contenthash record is
	// not valid.
	ErrInvalidContentHash = errors.New("invalid swarm content hash")
	// errNotImplemented denotes that the function has not been implemented.
	errNotImplemented = errors.New("function not implemented")
)

// Client is a name resolution client that can connect to ENS via an
// Ethereum endpoint.
type Client struct {
	endpoint  string
	ethCl     *ethclient.Client
	dialFn    func(string) (*ethclient.Client, error)
	resolveFn func(bind.ContractBackend, string) (string, error)
}

// Option is a function that applies an option to a Client.
type Option func(*Client)

// NewClient will return a new Client.
func NewClient(endpoint string, opts ...Option) (client.Interface, error) {
	c := &Client{
		endpoint:  endpoint,
		dialFn:    ethclient.Dial,
		resolveFn: wrapResolve,
	}

	// Apply all options to the Client.
	for _, o := range opts {
		o(c)
	}

	// Connect to the name resolution service.
	if c.dialFn == nil {
		return nil, fmt.Errorf("dialFn: %w", errNotImplemented)
	}

	ethCl, err := c.dialFn(c.endpoint)
	if err != nil {
		return nil, fmt.Errorf("%v: %w", err, ErrFailedToConnect)
	}
	c.ethCl = ethCl

	return c, nil
}

// IsConnected returns true if there is an active RPC connection with an
// Ethereum node at the configured endpoint.
func (c *Client) IsConnected() bool {
	return c.ethCl != nil
}

// Endpoint returns the endpoint the client was connected to.
func (c *Client) Endpoint() string {
	return c.endpoint
}

// Resolve implements the resolver.Client interface.
func (c *Client) Resolve(name string) (Address, error) {
	if c.resolveFn == nil {
		return swarm.ZeroAddress, fmt.Errorf("resolveFn: %w", errNotImplemented)
	}

	hash, err := c.resolveFn(c.ethCl, name)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("%v: %w", err, ErrResolveFailed)
	}

	// Ensure that the content hash string is in a valid format, eg.
	// "/swarm/<address>".
	if !strings.HasPrefix(hash, swarmContentHashPrefix) {
		return swarm.ZeroAddress, fmt.Errorf("contenthash %s: %w", hash, ErrInvalidContentHash)
	}

	// Trim the prefix and try to parse the result as a bzz address.
	return swarm.ParseHexAddress(strings.TrimPrefix(hash, swarmContentHashPrefix))
}

// Close closes the RPC connection with the client, terminating all unfinished
// requests. If the connection is already closed, this call is a noop.
func (c *Client) Close() error {
	if c.ethCl != nil {
		c.ethCl.Close()

	}
	c.ethCl = nil

	return nil
}

func wrapResolve(backend bind.ContractBackend, name string) (string, error) {

	// Connect to the ENS resolver for the provided name.
	ensR, err := goens.NewResolver(backend, name)
	if err != nil {
		return "", err
	}

	// Try and read out the content hash record.
	ch, err := ensR.Contenthash()
	if err != nil {
		return "", err
	}

	addr, err := goens.ContenthashToString(ch)
	if err != nil {
		return "", err
	}

	return addr, nil
}
