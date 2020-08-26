// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"github.com/ethersphere/bee/pkg/resolver"
)

// Assure mock Resolver implements the Resolver interface.
var _ resolver.Interface = (*Resolver)(nil)

// ErrNotImplemented denotes a function has not been implemented.
type ErrNotImplemented struct{}

// Error returns the formatted not implemented error.
func (e ErrNotImplemented) Error() string {
	return "function not implemented"
}

// Resolver is the mock Resolver implementation.
type Resolver struct {
	IsClosed    bool
	resolveFunc func(string) (resolver.Address, error)
}

// Option function sets the option on the mock Resolver.
type Option func(*Resolver)

// NewResolver will create a new mock Resolver.
func NewResolver(opts ...Option) resolver.Interface {
	r := &Resolver{}

	// Apply all options.
	for _, o := range opts {
		o(r)
	}

	return r
}

// WithResolveFunc will override the Resolve function implementation.
func WithResolveFunc(f func(string) (resolver.Address, error)) Option {
	return func(r *Resolver) {
		r.resolveFunc = f
	}
}

// Resolve implements the Resolver interface.
func (r *Resolver) Resolve(name string) (resolver.Address, error) {
	if r.resolveFunc != nil {
		return r.resolveFunc(name)
	}
	return resolver.Address{}, ErrNotImplemented{}
}

// Close implements the Resolver interface.
func (r *Resolver) Close() {
	r.IsClosed = true
}
