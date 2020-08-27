// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package resolver

import (
	"path"
	"strings"
)

// Ensure MultiResolver implements Resolver interface.
var _ Interface = (*MultiResolver)(nil)

type resolverMap map[string][]Interface

// MultiResolver performs name resolutions based on the TLD label in the name.
type MultiResolver struct {
	resolvers resolverMap
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

	return mr
}

// WithForceDefault will force resolution using the default resolver chain.
func WithForceDefault() Option {
	return func(mr *MultiResolver) {
		mr.ForceDefault = true
	}
}

// PushResolver will push a new Resolver to the name resolution chain for the
// given TLD.
// TLD names should be prepended with a dot (eg ".tld"). An empty TLD will push
// to the default resolver chain.
func (mr *MultiResolver) PushResolver(tld string, r Interface) error {
	if tld != "" && !isTLD(tld) {
		return ErrInvalidTLD("tld")
	}

	mr.resolvers[tld] = append(mr.resolvers[tld], r)
	return nil
}

// PopResolver will pop the last reslover from the name resolution chain for the
// given TLD.
// TLD names should be prepended with a dot (eg ".tld"). An empty TLD will pop
// from the default resolver chain.
func (mr *MultiResolver) PopResolver(tld string) error {
	if tld != "" && !isTLD(tld) {
		return ErrInvalidTLD(tld)
	}

	l := len(mr.resolvers[tld])
	if l == 0 {
		return ErrResolverChainEmpty(tld)
	}
	mr.resolvers[tld] = mr.resolvers[tld][:l-1]
	return nil
}

// ChainCount retruns the number of resolvers in a resolver chain for the given
// tld.
// TLD names should be prepended with a dot (eg ".tld"). An empty TLD will
// return the number of resolvers in the default resolver chain.
func (mr *MultiResolver) ChainCount(tld string) int {
	return len(mr.resolvers[tld])
}

// GetChain will return the resolution chain for a given TLD.
// TLD names should be prepended with a dot (eg ".tld"). An empty TLD will
// return all resolvers in the default resolver chain.
func (mr *MultiResolver) GetChain(tld string) []Interface {
	return mr.resolvers[tld]
}

// Resolve will attempt to resolve a name to an address.
// The resolution chain is selected based on the TLD of the name. If the name
// does not end in a TLD, the default resolution chain is selected.
// The resolution will be performed iteratively on the resolution chain,
// returning the result of the first Resolver that succeeds. If all resolvers
// in the chain return an error, the function will return an ErrResolveFailed.
func (mr *MultiResolver) Resolve(name string) (Address, error) {

	tld := ""
	if !mr.ForceDefault {
		tld = getTLD(name)
	}
	chain := mr.resolvers[tld]

	if len(chain) == 0 {
		return Address{}, ErrResolverChainEmpty(tld)
	}

	var err error
	for _, res := range chain {
		adr, err := res.Resolve(name)
		if err == nil {
			return adr, nil
		}
	}

	return Address{}, err
}

// Close all will call Close on all resolvers in all resolver chains.
func (mr *MultiResolver) Close() error {
	errs := new(CloseError)

	for _, chain := range mr.resolvers {
		for _, r := range chain {
			if err := r.Close(); err != nil {
				errs.add(err)
			}
		}
	}

	return errs.resolve()
}

func isTLD(tld string) bool {
	return len(tld) > 1 && tld[0] == '.'
}

func getTLD(name string) string {
	return path.Ext(strings.ToLower(name))
}
