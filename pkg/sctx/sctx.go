// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sctx provides convenience methods for context
// value injection and extraction.
package sctx

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethersphere/bee/pkg/tags"
)

var (
	// ErrTargetPrefix is returned when target prefix decoding fails.
	ErrTargetPrefix = errors.New("error decoding prefix string")
)

type (
	HTTPRequestIDKey struct{}
	requestHostKey   struct{}
	tagKey           struct{}
	gasPriceKey      struct{}
	gasLimitKey      struct{}
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

// SetTag sets the tag instance in the context
func SetTag(ctx context.Context, tagId *tags.Tag) context.Context {
	return context.WithValue(ctx, tagKey{}, tagId)
}

// GetTag gets the tag instance from the context
func GetTag(ctx context.Context) *tags.Tag {
	v, ok := ctx.Value(tagKey{}).(*tags.Tag)
	if !ok {
		return nil
	}
	return v
}

func SetGasLimit(ctx context.Context, limit uint64) context.Context {
	return context.WithValue(ctx, gasLimitKey{}, limit)
}

func GetGasLimit(ctx context.Context) uint64 {
	v, ok := ctx.Value(gasLimitKey{}).(uint64)
	if ok {
		return v
	}
	return 0
}

func GetGasLimitWithDefault(ctx context.Context, defaultLimit uint64) uint64 {
	limit := GetGasLimit(ctx)
	if limit == 0 {
		return defaultLimit
	}
	return limit
}

func SetGasPrice(ctx context.Context, price *big.Int) context.Context {
	return context.WithValue(ctx, gasPriceKey{}, price)

}

func GetGasPrice(ctx context.Context) *big.Int {
	v, ok := ctx.Value(gasPriceKey{}).(*big.Int)
	if ok {
		return v
	}
	return nil
}
