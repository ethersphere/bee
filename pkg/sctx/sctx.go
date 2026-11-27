// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sctx provides convenience methods for context
// value injection and extraction.
package sctx

import (
	"context"
	"errors"
)

// ErrTargetPrefix is returned when target prefix decoding fails.
var ErrTargetPrefix = errors.New("error decoding prefix string")

type (
	HTTPRequestIDKey struct{}
	requestHostKey   struct{}
)

// SetHost sets the http request host in the context
func SetHost(ctx context.Context, domain string) context.Context {
	return context.WithValue(ctx, requestHostKey{}, domain)
}

// GetHost gets the request host from the context
func GetHost(ctx context.Context) string {
	v, ok := ctx.Value(requestHostKey{}).(string)
	if ok {
		return v
	}
	return ""
}
