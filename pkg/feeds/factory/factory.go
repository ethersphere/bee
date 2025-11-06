// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"github.com/ethersphere/bee/v2/pkg/feeds"
	"github.com/ethersphere/bee/v2/pkg/feeds/epochs"
	"github.com/ethersphere/bee/v2/pkg/feeds/sequence"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
)

type factory struct {
	storage.Getter
}

func New(getter storage.Getter) feeds.Factory {
	return &factory{getter}
}

func (f *factory) NewLookup(t feeds.Type, feed *feeds.Feed, opts ...feeds.FactoryOption) (feeds.Lookup, error) {
	cfg := &feeds.FactoryConfig{Getter: f.Getter}

	for _, opt := range opts {
		opt(cfg)
	}

	switch t {
	case feeds.Sequence:
		return sequence.NewAsyncFinder(cfg.Getter, feed), nil
	case feeds.Epoch:
		return epochs.NewAsyncFinder(cfg.Getter, feed), nil
	}

	return nil, feeds.ErrFeedTypeNotFound
}
