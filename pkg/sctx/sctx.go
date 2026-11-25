// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sctx provides convenience methods for context
// value injection and extraction.
package sctx

import (
	"context"
	"errors"

	"github.com/ethersphere/bee/v2/pkg/transaction"
)

// ErrTargetPrefix is returned when target prefix decoding fails.
var ErrTargetPrefix = errors.New("error decoding prefix string")

type (
	HTTPRequestIDKey struct{}
	requestHostKey   struct{}
	txOptionsKey     struct{}
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

func SetTxOptions(ctx context.Context, opts []transaction.Option) context.Context {
	return context.WithValue(ctx, txOptionsKey{}, opts)
}

func TxOptionsFrom(ctx context.Context) []transaction.Option {
	opts, _ := ctx.Value(txOptionsKey{}).([]transaction.Option)
	return opts
}
