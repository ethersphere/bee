// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package multiresolver

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/resolver"
	"github.com/ethersphere/bee/v2/pkg/resolver/cidv1"
	"github.com/ethersphere/bee/v2/pkg/resolver/client/ens"
	"github.com/hashicorp/go-multierror"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "multiresolver"

// Ensure MultiResolver implements Resolver interface.
var _ resolver.Interface = (*MultiResolver)(nil)

var (
	// ErrTLDTooLong denotes when a TLD in a name exceeds maximum length.
	ErrTLDTooLong = fmt.Errorf("TLD exceeds maximum length of %d characters", maxTLDLength)
	// ErrInvalidTLD denotes passing an invalid TLD to the MultiResolver.
	ErrInvalidTLD = errors.New("invalid TLD")
	// ErrResolverChainEmpty denotes trying to pop an empty resolver chain.
	ErrResolverChainEmpty = errors.New("resolver chain empty")
	// ErrResolverChainFailed denotes that an entire name resolution chain
	// for a given TLD failed.
	ErrResolverChainFailed = errors.New("resolver chain failed")
	// ErrCloseFailed denotes that closing the multiresolver failed.
	ErrCloseFailed = errors.New("close failed")
)

type resolverMap map[string][]resolver.Interface

// MultiResolver performs name resolutions based on the TLD label in the name.
type MultiResolver struct {
	resolvers resolverMap
	logger    log.Logger
	cfgs      []ConnectionConfig
	// ForceDefault will force all names to be resolved by the default
	// resolution chain, regadless of their TLD.
	ForceDefault bool
}

// Option is a function that applies an option to a MultiResolver.
type Option func(*MultiResolver)

// NewMultiResolver will return a new MultiResolver instance.
func NewMultiResolver(opts ...Option) *MultiResolver {
	mr := &MultiResolver{
		resolvers: make(resolverMap),
	}

	// Apply all options.
	for _, o := range opts {
		o(mr)
	}

	// Discard log output by default.
	if mr.logger == nil {
		mr.logger = log.Noop
	}
	log := mr.logger

	if len(mr.cfgs) == 0 {
		log.Info("name resolver: no name resolution service provided")
		return mr
	}

	// Attempt to connect to each resolver using the connection string.
	for _, c := range mr.cfgs {

		// NOTE: if we want to create a specific client based on the TLD
		// we can do it here.
		mr.connectENSClient(c.TLD, c.Address, c.Endpoint)
	}

	return mr
}

// WithConnectionConfigs will set the initial connection configuration.
func WithConnectionConfigs(cfgs []ConnectionConfig) Option {
	return func(mr *MultiResolver) {
		mr.cfgs = cfgs
	}
}

// WithLogger will set the logger used by the MultiResolver.
func WithLogger(logger log.Logger) Option {
	return func(mr *MultiResolver) {
		mr.logger = logger.WithName(loggerName).Register()
	}
}

// WithForceDefault will force resolution using the default resolver chain.
func WithForceDefault() Option {
	return func(mr *MultiResolver) {
		mr.ForceDefault = true
	}
}

func WithDefaultCIDResolver() Option {
	return func(mr *MultiResolver) {
		mr.PushResolver("", cidv1.Resolver{})
	}
}

// PushResolver will push a new Resolver to the name resolution chain for the
// given TLD. An empty TLD will push to the default resolver chain.
func (mr *MultiResolver) PushResolver(tld string, r resolver.Interface) {
	mr.resolvers[tld] = append(mr.resolvers[tld], r)
}

// PopResolver will pop the last resolver from the name resolution chain for the
// given TLD. An empty TLD will pop from the default resolver chain.
func (mr *MultiResolver) PopResolver(tld string) error {
	l := len(mr.resolvers[tld])
	if l == 0 {
		return fmt.Errorf("tld %s: %w", tld, ErrResolverChainEmpty)
	}
	mr.resolvers[tld] = mr.resolvers[tld][:l-1]
	return nil
}

// ChainCount returns the number of resolvers in a resolver chain for the given
// tld.
// TLD names should be prepended with a dot (eg ".tld"). An empty TLD will
// return the number of resolvers in the default resolver chain.
func (mr *MultiResolver) ChainCount(tld string) int {
	return len(mr.resolvers[tld])
}

// GetChain will return the resolution chain for a given TLD.
// TLD names should be prepended with a dot (eg ".tld"). An empty TLD will
// return all resolvers in the default resolver chain.
func (mr *MultiResolver) GetChain(tld string) []resolver.Interface {
	return mr.resolvers[tld]
}

// Resolve will attempt to resolve a name to an address.
// The resolution chain is selected based on the TLD of the name. If the name
// does not end in a TLD, the default resolution chain is selected.
// The resolution will be performed iteratively on the resolution chain,
// returning the result of the first Resolver that succeeds. If all resolvers
// in the chain return an error, the function will return an ErrResolveFailed.
func (mr *MultiResolver) Resolve(name string) (addr resolver.Address, err error) {
	tld := ""
	if !mr.ForceDefault {
		tld = getTLD(name)
	}
	chain := mr.resolvers[tld]

	// If no resolver chain is found, switch to the default chain.
	if len(chain) == 0 {
		chain = mr.resolvers[""]
	}

	var errs *multierror.Error
	for _, res := range chain {
		addr, err = res.Resolve(name)
		if err == nil {
			return addr, nil
		}
		errs = multierror.Append(err)
	}

	return addr, errs.ErrorOrNil()
}

// Close all will call Close on all resolvers in all resolver chains.
func (mr *MultiResolver) Close() error {
	var errs *multierror.Error

	for _, chain := range mr.resolvers {
		for _, r := range chain {
			if err := r.Close(); err != nil {
				errs = multierror.Append(err)
			}
		}
	}

	return errs.ErrorOrNil()
}

func getTLD(name string) string {
	return path.Ext(strings.ToLower(name))
}

func (mr *MultiResolver) connectENSClient(tld, address, endpoint string) {
	log := mr.logger

	if address == "" {
		log.Debug("connecting to endpoint", "tld", tld, "endpoint", endpoint)
	} else {
		log.Debug("connecting to endpoint with contract address", "tld", tld, "endpoint", endpoint, "contract_address", address)
	}

	ensCl, err := ens.NewClient(endpoint, ens.WithContractAddress(address))
	if err != nil {
		log.Error(err, "resolver on endpoint failed", "tld", tld, "endpoint", endpoint)
	} else {
		log.Info("connected", "tld", tld, "endpoint", endpoint)
		mr.PushResolver(tld, ensCl)
	}
}
