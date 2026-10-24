// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ens

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	goens "github.com/wealdtech/go-ens/v3"

	"github.com/ethersphere/bee/v2/pkg/resolver"
	"github.com/ethersphere/bee/v2/pkg/resolver/client"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	defaultENSContractAddress = "00000000000C2E074eC69A0dFb2997BA6C7d2e1e"
	swarmContentHashPrefix    = "bzz://"

	// probeTimeout bounds the extra call made when the configured contract is
	// not an ENS registry.
	probeTimeout = 10 * time.Second
)

var (
	// supportsInterfaceSelector is the EIP-165 supportsInterface(bytes4) selector.
	supportsInterfaceSelector = []byte{0x01, 0xff, 0xc9, 0xa7}
	// contenthashInterfaceID is the EIP-165 id of the ENS contenthash resolver
	// profile (ENSIP-7), contenthash(bytes32).
	contenthashInterfaceID = []byte{0xbc, 0x1c, 0x58, 0xd1}
)

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
	// ErrNotImplemented denotes that the function has not been implemented.
	ErrNotImplemented = errors.New("function not implemented")
	// errNameNotRegistered denotes that the name is not registered.
	errNameNotRegistered = errors.New("name is not registered")
)

// Client is a name resolution client that can connect to ENS via an
// Ethereum endpoint.
//
// The configured contract is normally an ENS registry. It may instead be a
// contract that implements the ENS resolver profile itself (contenthash,
// addr, text on EIP-137 name hashes) without a registry in front, as some
// ENS-compatible name services do; such a contract is detected when it is
// dialled, and names are then resolved by asking it for the content hash
// directly.
type Client struct {
	endpoint        string
	contractAddr    string
	ethCl           *ethclient.Client
	connectFn       func(string, string) (*ethclient.Client, *goens.Registry, error)
	resolveFn       func(*goens.Registry, common.Address, string) (string, error)
	resolveDirectFn func(*ethclient.Client, common.Address, string) (string, error)
	registry        *goens.Registry
	// direct is set when the contract is a resolver rather than a registry.
	direct bool
}

// Option is a function that applies an option to a Client.
type Option func(*Client)

// NewClient will return a new Client.
func NewClient(endpoint string, opts ...Option) (client.Interface, error) {
	c := &Client{
		endpoint:        endpoint,
		connectFn:       wrapDial,
		resolveFn:       wrapResolve,
		resolveDirectFn: wrapResolveDirect,
	}

	// Apply all options to the Client.
	for _, o := range opts {
		o(c)
	}

	// Set the default ENS contract address.
	if c.contractAddr == "" {
		c.contractAddr = defaultENSContractAddress
	}

	// Establish a connection to the ENS.
	if c.connectFn == nil {
		return nil, fmt.Errorf("connectFn: %w", ErrNotImplemented)
	}
	ethCl, registry, err := c.connectFn(c.endpoint, c.contractAddr)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", err, ErrFailedToConnect)
	}
	c.ethCl = ethCl
	c.registry = registry
	// A live connection without a registry means the contract answered the
	// resolver-profile probe in the dial function.
	c.direct = registry == nil && ethCl != nil

	return c, nil
}

// WithContractAddress will set the ENS contract address.
func WithContractAddress(addr string) Option {
	return func(c *Client) {
		c.contractAddr = addr
	}
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
		return swarm.ZeroAddress, fmt.Errorf("resolveFn: %w", ErrNotImplemented)
	}

	var hash string
	var err error
	if c.direct {
		if c.resolveDirectFn == nil {
			return swarm.ZeroAddress, fmt.Errorf("resolveDirectFn: %w", ErrNotImplemented)
		}
		hash, err = c.resolveDirectFn(c.ethCl, common.HexToAddress(c.contractAddr), name)
	} else {
		hash, err = c.resolveFn(c.registry, common.HexToAddress(c.contractAddr), name)
	}
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("%w: %w", err, ErrResolveFailed)
	}

	// Ensure that the content hash string is in a valid format, eg.
	// "bzz://<address>".
	if !strings.HasPrefix(hash, swarmContentHashPrefix) {
		return swarm.ZeroAddress, fmt.Errorf("check content hash prefix %s: %w", hash, resolver.ErrInvalidContentHash)
	}

	// Trim the prefix and try to parse the result as a bzz address.
	addr, err := swarm.ParseHexAddress(strings.TrimPrefix(hash, swarmContentHashPrefix))
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("parse response hash %s: %w", hash, resolver.ErrInvalidContentHash)
	}

	return addr, nil
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

func wrapDial(endpoint, contractAddr string) (*ethclient.Client, *goens.Registry, error) {
	// Dial the eth client.
	ethCl, err := ethclient.Dial(endpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("dial: %w", err)
	}

	// Obtain the ENS registry.
	registry, err := goens.NewRegistryAt(ethCl, common.HexToAddress(contractAddr))
	if err != nil {
		return nil, nil, fmt.Errorf("new registry: %w", err)
	}

	// Ensure that the ENS registry client is deployed to the given contract address.
	_, err = registry.Owner("")
	if err != nil {
		// Not a registry. The contract may still be a resolver that serves the
		// ENS resolver profile directly (no registry in front); in that case
		// return a nil registry and resolve against the contract itself.
		ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
		defer cancel()
		isResolver, probeErr := supportsContenthash(ctx, ethCl, common.HexToAddress(contractAddr))
		if probeErr == nil && isResolver {
			return ethCl, nil, nil
		}
		return nil, nil, fmt.Errorf("owner: %w", err)
	}

	return ethCl, registry, nil
}

// contractCaller is the part of an Ethereum client needed for the EIP-165
// probe; it lets tests use a fake instead of a live connection.
type contractCaller interface {
	CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
}

// supportsContenthash reports whether the contract at addr answers true to
// EIP-165 supportsInterface for the ENS contenthash resolver profile.
func supportsContenthash(ctx context.Context, caller contractCaller, addr common.Address) (bool, error) {
	// supportsInterface(bytes4): selector, then the 4-byte id left-aligned in
	// a 32-byte word.
	data := make([]byte, 0, 4+32)
	data = append(data, supportsInterfaceSelector...)
	data = append(data, contenthashInterfaceID...)
	data = append(data, make([]byte, 28)...)
	out, err := caller.CallContract(ctx, ethereum.CallMsg{To: &addr, Data: data}, nil)
	if err != nil {
		return false, err
	}
	// A bool return is a 32-byte word; anything else means the contract does
	// not implement EIP-165 (or does not exist).
	if len(out) != 32 {
		return false, nil
	}
	return out[31] == 1, nil
}

// wrapResolveDirect reads the content hash from a contract that implements
// the ENS resolver profile for the name itself, without a registry lookup.
// An unregistered name has no record and yields an empty content hash.
func wrapResolveDirect(ethCl *ethclient.Client, contractAddr common.Address, name string) (string, error) {
	ensR, err := goens.NewResolverAt(ethCl, name, contractAddr)
	if err != nil {
		return "", fmt.Errorf("%w: %w", resolver.ErrServiceNotAvailable, err)
	}

	ch, err := ensR.Contenthash()
	if err != nil {
		if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "rate limit") {
			return "", fmt.Errorf("%w: %w", resolver.ErrServiceNotAvailable, err)
		}
		return "", fmt.Errorf("contenthash: %w: %w", err, resolver.ErrInvalidContentHash)
	}
	if len(ch) == 0 {
		return "", fmt.Errorf("%w: %w", errNameNotRegistered, resolver.ErrNotFound)
	}

	addr, err := goens.ContenthashToString(ch)
	if err != nil {
		return "", fmt.Errorf("contenthash to string: %w: %w", err, resolver.ErrInvalidContentHash)
	}

	return addr, nil
}

func wrapResolve(registry *goens.Registry, _ common.Address, name string) (string, error) {
	ownerAddress, err := registry.Owner(name)
	// it returns error only if the service is not available
	if err != nil {
		return "", fmt.Errorf("%w: %w", resolver.ErrServiceNotAvailable, err)
	}

	// If the name is not registered, return an error.
	if bytes.Equal(ownerAddress.Bytes(), goens.UnknownAddress.Bytes()) {
		return "", fmt.Errorf("%w: %w", errNameNotRegistered, resolver.ErrNotFound)
	}

	// Obtain the resolver for this domain name.
	ensR, err := registry.Resolver(name)
	if err != nil {
		return "", fmt.Errorf("%w: %w", resolver.ErrServiceNotAvailable, err)
	}

	// Try and read out the content hash record.
	ch, err := ensR.Contenthash()
	if err != nil {
		// Check if it's a service error (rate limiting, network issues)
		if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "rate limit") {
			return "", fmt.Errorf("%w: %w", resolver.ErrServiceNotAvailable, err)
		}
		return "", fmt.Errorf("contenthash: %w: %w", err, resolver.ErrInvalidContentHash)
	}

	addr, err := goens.ContenthashToString(ch)
	if err != nil {
		return "", fmt.Errorf("contenthash to string: %w: %w", err, resolver.ErrInvalidContentHash)
	}

	return addr, nil
}
