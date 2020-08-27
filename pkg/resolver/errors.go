// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package resolver

import (
	"fmt"
	"strings"
)

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

// MultiCloseError denotes that at least one resolver in the MultiResolver has
// had an error when Close was called.
type MultiCloseError struct {
	errs []error
}

func (me MultiCloseError) add(err error) {
	me.errs = append(me.errs, err)
}

func (me MultiCloseError) resolve() error {
	if len(me.errs) > 0 {
		return me
	}
	return nil
}

// Error returns a formatted multi close error.
func (me MultiCloseError) Error() string {
	if len(me.errs) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("multiresolver failed to close: ")

	for _, e := range me.errs {
		b.WriteString(e.Error())
		b.WriteString("; ")
	}

	return b.String()
}
