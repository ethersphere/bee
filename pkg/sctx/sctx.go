// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.package storage

package sctx

import (
	"context"

	"encoding/hex"
	"errors"
	"strings"

	"github.com/ethersphere/bee/pkg/tags"

	"github.com/ethersphere/bee/pkg/trojan"
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
func GetTargets(ctx context.Context) (trojan.Targets, error) {
	targetString, ok := ctx.Value(targetsContextKey{}).(string)
	if !ok {
		return nil, ErrTargetPrefix
	}

	prefixes := strings.Split(targetString, ",")
	var targets trojan.Targets
	for _, prefix := range prefixes {
		var target trojan.Target
		target, err := hex.DecodeString(prefix)
		if err != nil {
			continue
		}
		targets = append(targets, target)
	}
	if len(targets) <= 0 {
		return nil, ErrTargetPrefix
	}
	return targets, nil
}
