// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package resolver

import "fmt"

// ErrInvalidTLD denotes passing an invalid TLD to the MultiResolver.
type ErrInvalidTLD string

// Error returns the formatted invalid TLD error.
func (e ErrInvalidTLD) Error() string {
	return fmt.Sprintf("Invalid TLD %q", string(e))
}

// ErrResolverChainEmpty denotes trying to pop an empty resolver chain.
type ErrResolverChainEmpty string

// Error returns the formatted resolver chain empty error.
func (e ErrResolverChainEmpty) Error() string {
	return fmt.Sprintf("Resolver chain for %q empty", string(e))
}
