// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ethersphere/bee/v2/pkg/feeds"
	"github.com/ethersphere/bee/v2/pkg/feeds/epochs"
	"github.com/ethersphere/bee/v2/pkg/feeds/sequence"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type factory struct {
	storage.Getter
	metrics metrics
}

func New(getter storage.Getter) feeds.Factory {
	return &factory{
		Getter:  getter,
		metrics: newMetrics(),
	}
}

func (f *factory) NewLookup(t feeds.Type, feed *feeds.Feed) (feeds.Lookup, error) {
	var lookup feeds.Lookup
	switch t {
	case feeds.Sequence:
		lookup = sequence.NewAsyncFinder(f.Getter, feed)
	case feeds.Epoch:
		lookup = epochs.NewAsyncFinder(f.Getter, feed)
	default:
		return nil, feeds.ErrFeedTypeNotFound
	}

	return f.wrapLookup(t, lookup), nil
}

func (f *factory) wrapLookup(t feeds.Type, lookup feeds.Lookup) feeds.Lookup {
	return &instrumentedLookup{
		lookup: lookup,
		typ:    strings.ToLower(t.String()),
		m:      f.metrics,
	}
}

type instrumentedLookup struct {
	lookup feeds.Lookup
	typ    string
	m      metrics
}

func (l *instrumentedLookup) At(ctx context.Context, at int64, after uint64) (swarm.Chunk, feeds.Index, feeds.Index, error) {
	l.m.LookupStarted.WithLabelValues(l.typ).Inc()
	start := time.Now()
	ch, cur, next, err := l.lookup.At(ctx, at, after)
	l.m.LookupDuration.WithLabelValues(l.typ, lookupResult(ch, err)).Observe(time.Since(start).Seconds())
	return ch, cur, next, err
}

func lookupResult(ch swarm.Chunk, err error) string {
	switch {
	case err != nil && errors.Is(err, context.Canceled):
		return "canceled"
	case err != nil:
		return "error"
	case ch == nil:
		return "not_found"
	default:
		return "found"
	}
}
