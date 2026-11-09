// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ens

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	goens "github.com/wealdtech/go-ens/v3"

	"github.com/ethersphere/bee/v2/pkg/resolver"
	"github.com/ethersphere/bee/v2/pkg/resolver/client"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/abiutil"
)

const (
	// defaultENSContractAddress is the ENS Universal Resolver proxy. It
	// resolves both ENSv1 and ENSv2 names, unlike the legacy ENSv1 registry
	// which has no resolver set for names migrated to ENSv2.
	defaultENSContractAddress = "eEeEEEeE14D718C2B47D9923Deab1335E144EeEe"
	swarmContentHashPrefix    = "bzz://"
	// maxDNSLabelLength is the maximum length of a DNS wire format label.
	maxDNSLabelLength = 255
	// maxDNSNameLength is the maximum length of a DNS wire format name,
	// including length prefixes and the root terminator.
	maxDNSNameLength = 255
)

// universalResolverABI contains the subset of the ENS Universal Resolver
// interface used to resolve contenthash records, together with the
// contenthash resolver profile that is passed through it.
var universalResolverABI = abiutil.MustParseABI(`[
	{"type":"function","name":"resolve","stateMutability":"view","inputs":[{"name":"name","type":"bytes"},{"name":"data","type":"bytes"}],"outputs":[{"name":"","type":"bytes"},{"name":"","type":"address"}]},
	{"type":"function","name":"contenthash","stateMutability":"view","inputs":[{"name":"node","type":"bytes32"}],"outputs":[{"name":"","type":"bytes"}]},
	{"type":"error","name":"ResolverNotFound","inputs":[{"name":"name","type":"bytes"}]},
	{"type":"error","name":"ResolverNotContract","inputs":[{"name":"name","type":"bytes"},{"name":"resolver","type":"address"}]},
	{"type":"error","name":"UnsupportedResolverProfile","inputs":[{"name":"selector","type":"bytes4"}]},
	{"type":"error","name":"ResolverError","inputs":[{"name":"errorData","type":"bytes"}]},
	{"type":"error","name":"OffchainLookup","inputs":[{"name":"sender","type":"address"},{"name":"urls","type":"string[]"},{"name":"callData","type":"bytes"},{"name":"callbackFunction","type":"bytes4"},{"name":"extraData","type":"bytes"}]}
]`)

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
	// errInvalidName denotes that the name can not be DNS-encoded.
	errInvalidName = errors.New("invalid name")
	// errOffchainLookup denotes that the name requires an ERC-3668 (CCIP-Read)
	// offchain lookup, which is not supported.
	errOffchainLookup = errors.New("offchain lookup is not supported")
)

// Client is a name resolution client that can connect to ENS via an
// Ethereum endpoint.
type Client struct {
	endpoint     string
	contractAddr string
	ethCl        *ethclient.Client
	connectFn    func(string, string) (*ethclient.Client, error)
	resolveFn    func(*ethclient.Client, common.Address, string) (string, error)
}

// Option is a function that applies an option to a Client.
type Option func(*Client)

// NewClient will return a new Client.
func NewClient(endpoint string, opts ...Option) (client.Interface, error) {
	c := &Client{
		endpoint:  endpoint,
		connectFn: wrapDial,
		resolveFn: wrapResolve,
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
	ethCl, err := c.connectFn(c.endpoint, c.contractAddr)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", err, ErrFailedToConnect)
	}
	c.ethCl = ethCl

	return c, nil
}

// WithContractAddress will set the ENS Universal Resolver contract address.
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

	hash, err := c.resolveFn(c.ethCl, common.HexToAddress(c.contractAddr), name)
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

func wrapDial(endpoint, contractAddr string) (*ethclient.Client, error) {
	// Dial the eth client.
	ethCl, err := ethclient.Dial(endpoint)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}

	// Ensure that a contract is deployed to the given Universal Resolver address.
	code, err := ethCl.CodeAt(context.Background(), common.HexToAddress(contractAddr), nil)
	if err != nil {
		ethCl.Close()
		return nil, fmt.Errorf("code at %s: %w", contractAddr, err)
	}
	if len(code) == 0 {
		ethCl.Close()
		return nil, fmt.Errorf("no contract code at %s", contractAddr)
	}

	return ethCl, nil
}

func wrapResolve(ethCl *ethclient.Client, contractAddr common.Address, name string) (string, error) {
	normalized, err := goens.Normalize(name)
	if err != nil {
		return "", fmt.Errorf("normalize: %w: %w", err, resolver.ErrNotFound)
	}
	dnsName, err := dnsEncode(normalized)
	if err != nil {
		return "", fmt.Errorf("dns encode: %w: %w", err, resolver.ErrNotFound)
	}
	node, err := goens.NameHash(normalized)
	if err != nil {
		return "", fmt.Errorf("namehash: %w: %w", err, resolver.ErrNotFound)
	}

	// The Universal Resolver finds the resolver of the name, both for ENSv1
	// and ENSv2 names, and forwards the contenthash call to it.
	data, err := universalResolverABI.Pack("contenthash", node)
	if err != nil {
		return "", fmt.Errorf("pack contenthash: %w", err)
	}
	input, err := universalResolverABI.Pack("resolve", dnsName, data)
	if err != nil {
		return "", fmt.Errorf("pack resolve: %w", err)
	}

	output, err := ethCl.CallContract(context.Background(), ethereum.CallMsg{To: &contractAddr, Data: input}, nil)
	if err != nil {
		return "", resolveError(err)
	}

	results, err := universalResolverABI.Unpack("resolve", output)
	if err != nil {
		return "", fmt.Errorf("unpack resolve: %w: %w", err, resolver.ErrInvalidContentHash)
	}
	result, ok := results[0].([]byte)
	if !ok {
		return "", fmt.Errorf("unexpected resolve result type %T: %w", results[0], resolver.ErrInvalidContentHash)
	}
	chResults, err := universalResolverABI.Unpack("contenthash", result)
	if err != nil {
		return "", fmt.Errorf("unpack contenthash: %w: %w", err, resolver.ErrInvalidContentHash)
	}
	ch, ok := chResults[0].([]byte)
	if !ok {
		return "", fmt.Errorf("unexpected contenthash type %T: %w", chResults[0], resolver.ErrInvalidContentHash)
	}

	addr, err := goens.ContenthashToString(ch)
	if err != nil {
		return "", fmt.Errorf("contenthash to string: %w: %w", err, resolver.ErrInvalidContentHash)
	}

	return addr, nil
}

// resolveError classifies an error returned by the Universal Resolver call.
// Known contract reverts are caused by the name or its records, while any
// other error is treated as the endpoint not being available.
func resolveError(err error) error {
	buf, ok := revertData(err)
	if !ok {
		return fmt.Errorf("%w: %w", resolver.ErrServiceNotAvailable, err)
	}

	switch {
	case isRevert(buf, "ResolverNotFound"), isRevert(buf, "ResolverNotContract"):
		return fmt.Errorf("%w: %w", errNameNotRegistered, resolver.ErrNotFound)
	case isRevert(buf, "OffchainLookup"):
		return fmt.Errorf("%w: %w", errOffchainLookup, resolver.ErrServiceNotAvailable)
	case isRevert(buf, "UnsupportedResolverProfile"), isRevert(buf, "ResolverError"):
		return fmt.Errorf("contenthash: %w: %w", err, resolver.ErrInvalidContentHash)
	}

	return fmt.Errorf("%w: %w", resolver.ErrServiceNotAvailable, err)
}

// revertData extracts ABI revert data from an eth_call error.
func revertData(err error) ([]byte, bool) {
	if data, ok := ethclient.RevertErrorData(err); ok {
		return data, true
	}
	var derr rpc.DataError
	if !errors.As(err, &derr) {
		return nil, false
	}
	switch data := derr.ErrorData().(type) {
	case string:
		buf, decErr := hexutil.Decode(data)
		if decErr != nil {
			return nil, false
		}
		return buf, true
	case []byte:
		return data, true
	case hexutil.Bytes:
		return data, true
	default:
		return nil, false
	}
}

// isRevert reports whether the revert data starts with the selector of the
// named Universal Resolver error.
func isRevert(data []byte, name string) bool {
	id := universalResolverABI.Errors[name].ID
	return len(data) >= 4 && bytes.Equal(data[:4], id[:4])
}

// dnsEncode encodes the name in the DNS wire format expected by the Universal
// Resolver, eg. "swarm.eth" becomes "\x05swarm\x03eth\x00".
func dnsEncode(name string) ([]byte, error) {
	if name == "" {
		return nil, errInvalidName
	}

	labels := strings.Split(name, ".")
	buf := make([]byte, 0, len(name)+2)
	for _, label := range labels {
		if len(label) == 0 || len(label) > maxDNSLabelLength {
			return nil, fmt.Errorf("label length %d: %w", len(label), errInvalidName)
		}
		if len(buf)+1+len(label)+1 > maxDNSNameLength {
			return nil, fmt.Errorf("name length %d: %w", len(buf)+1+len(label)+1, errInvalidName)
		}
		buf = append(buf, byte(len(label)))
		buf = append(buf, label...)
	}

	return append(buf, 0), nil
}
