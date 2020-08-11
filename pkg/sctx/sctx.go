// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.package storage

package sctx

import (
	"context"

	"github.com/ethersphere/bee/pkg/tags"
)

type (
	HTTPRequestIDKey struct{}
	requestHostKey   struct{}
	tagKey           struct{}
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
