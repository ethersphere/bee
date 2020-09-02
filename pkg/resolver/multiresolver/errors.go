// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package multiresolver

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrTLDTooLong denotes when a TLD in a name exceeds maximum length.
	ErrTLDTooLong = fmt.Errorf("TLD exceeds maximum length of %d characters", maxTLDLength)
	// ErrInvalidTLD denotes passing an invalid TLD to the MultiResolver.
	ErrInvalidTLD = errors.New("invalid TLD")
	// ErrResolverChainEmpty denotes trying to pop an empty resolver chain.
	ErrResolverChainEmpty = errors.New("resolver chain empty")
)

// CloseError denotes that at least one resolver in the MultiResolver has
// had an error when Close was called.
type CloseError struct {
	errs []error
}

func (me CloseError) add(err error) {
	if err != nil {
		me.errs = append(me.errs, err)
	}
}

func (me CloseError) errorOrNil() error {
	if len(me.errs) > 0 {
		return me
	}
	return nil
}

// Error returns a formatted multi close error.
func (me CloseError) Error() string {
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
