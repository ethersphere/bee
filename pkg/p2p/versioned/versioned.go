// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package versioned provides helpers for constructing versioned P2P stream handlers.
package versioned

import (
	"context"
	"fmt"
	"sort"

	"github.com/coreos/go-semver/semver"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/prometheus/client_golang/prometheus"
)

// Handler represents a p2p.HandlerFunc associated with a minimum supported Version threshold.
type Handler struct {
	Version *semver.Version
	Handler p2p.HandlerFunc
}

// Option defines a functional option for configuring NewHandlersFunc behavior.
type Option interface {
	apply(*config)
}

type optionFunc func(*config)

func (f optionFunc) apply(c *config) {
	f(c)
}

type config struct {
	counter *prometheus.CounterVec
	onMatch func(version *semver.Version)
}

// WithMetricCounter configures a prometheus.CounterVec (labeled by "version") to be incremented when a version handler is matched.
func WithMetricCounter(counter *prometheus.CounterVec) Option {
	return optionFunc(func(c *config) {
		c.counter = counter
	})
}

// WithOnMatchFunc configures a custom callback function that is invoked with the matched version.
func WithOnMatchFunc(fn func(version *semver.Version)) Option {
	return optionFunc(func(c *config) {
		c.onMatch = fn
	})
}

// NewHandlersFunc creates a new p2p.HandlerFunc that dispatches stream execution
// based on the stream version.
//
// Handlers are evaluated in descending order of Version. The first handler where
// stream.Version >= handler.Version will be executed.
func NewHandlersFunc(handlers []Handler, opts ...Option) p2p.HandlerFunc {
	sorted := make([]Handler, len(handlers))
	copy(sorted, handlers)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[j].Version.LessThan(*sorted[i].Version)
	})

	cfg := &config{}
	for _, opt := range opts {
		opt.apply(cfg)
	}

	return func(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
		v, err := stream.Version()
		if err != nil {
			return fmt.Errorf("get stream version: %w", err)
		}

		for _, h := range sorted {
			if !v.LessThan(*h.Version) {
				if cfg.counter != nil {
					cfg.counter.WithLabelValues(h.Version.String()).Inc()
				}
				if cfg.onMatch != nil {
					cfg.onMatch(h.Version)
				}
				return h.Handler(ctx, p, stream)
			}
		}

		return fmt.Errorf("no handler found for stream version: %s", v.String())
	}
}
