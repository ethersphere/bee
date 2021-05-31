// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sctx provides convenience methods for context
// value injection and extraction.
package sctx

import (
	"context"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"

	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/tags"
)

var (
	// ErrTargetPrefix is returned when target prefix decoding fails.
	ErrTargetPrefix = errors.New("error decoding prefix string")
)

type (
	HTTPRequestIDKey  struct{}
	requestHostKey    struct{}
	tagKey            struct{}
	targetsContextKey struct{}
	gasPriceKey       struct{}
	gasLimitKey       struct{}
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

// SetTargets set the target string in the context to be used downstream in netstore
func SetTargets(ctx context.Context, targets string) context.Context {
	return context.WithValue(ctx, targetsContextKey{}, targets)
}

// GetTargets returns the specific target pinners for a corresponding chunk by
// reading the prefix targets sent in the download API.
func GetTargets(ctx context.Context) pss.Targets {
	targetString, ok := ctx.Value(targetsContextKey{}).(string)
	if !ok {
		return nil
	}

	prefixes := strings.Split(targetString, ",")
	var targets pss.Targets
	for _, prefix := range prefixes {
		var target pss.Target
		target, err := hex.DecodeString(prefix)
		if err != nil {
			continue
		}
		targets = append(targets, target)
	}
	if len(targets) <= 0 {
		return nil
	}
	return targets
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
