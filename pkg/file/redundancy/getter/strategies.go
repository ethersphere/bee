// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package getter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/retrieval"
)

const (
	DefaultStrategy        = NONE                           // default prefetching strategy
	DefaultStrict          = true                           // default fallback modes
	DefaultFetchTimeout    = retrieval.RetrieveChunkTimeout // timeout for each chunk retrieval
	DefaultStrategyTimeout = 300 * time.Millisecond         // timeout for each strategy
)

type (
	strategyKey        struct{}
	modeKey            struct{}
	fetchTimeoutKey    struct{}
	strategyTimeoutKey struct{}
	loggerKey          struct{}
	Strategy           = int
)

// Config is the configuration for the getter - public
type Config struct {
	Strategy        Strategy
	Strict          bool
	FetchTimeout    time.Duration
	StrategyTimeout time.Duration
	Logger          log.Logger
}

const (
	NONE Strategy = iota // no prefetching and no decoding
	DATA                 // just retrieve data shards no decoding
	PROX                 // proximity driven selective fetching
	RACE                 // aggressive fetching racing all chunks
	strategyCnt
)

// DefaultConfig is the default configuration for the getter
var DefaultConfig = Config{
	Strategy:        DefaultStrategy,
	Strict:          DefaultStrict,
	FetchTimeout:    DefaultFetchTimeout,
	StrategyTimeout: DefaultStrategyTimeout,
	Logger:          log.Noop,
}

// NewConfigFromContext returns a new Config based on the context
func NewConfigFromContext(ctx context.Context, def Config) (conf Config, err error) {
	var ok bool
	conf = def
	e := func(s string, errs ...error) error {
		if len(errs) > 0 {
			return fmt.Errorf("error setting %s from context: %w", s, errors.Join(errs...))
		}
		return fmt.Errorf("error setting %s from context", s)
	}
	if val := ctx.Value(strategyKey{}); val != nil {
		conf.Strategy, ok = val.(Strategy)
		if !ok {
			return conf, e("strategy")
		}
	}
	if val := ctx.Value(modeKey{}); val != nil {
		conf.Strict, ok = val.(bool)
		if !ok {
			return conf, e("fallback mode")
		}
	}
	if val := ctx.Value(fetchTimeoutKey{}); val != nil {
		conf.FetchTimeout, ok = val.(time.Duration)
		if !ok {
			return conf, e("fetcher timeout")
		}
	}
	if val := ctx.Value(strategyTimeoutKey{}); val != nil {
		conf.StrategyTimeout, ok = val.(time.Duration)
		if !ok {
			return conf, e("strategy timeout")
		}
	}
	if val := ctx.Value(loggerKey{}); val != nil {
		conf.Logger, ok = val.(log.Logger)
		if !ok {
			return conf, e("strategy timeout")
		}
	}

	return conf, nil
}

// SetStrategy sets the strategy for the retrieval
func SetStrategy(ctx context.Context, s Strategy) context.Context {
	return context.WithValue(ctx, strategyKey{}, s)
}

// SetStrict sets the strict mode for the retrieval
func SetStrict(ctx context.Context, strict bool) context.Context {
	return context.WithValue(ctx, modeKey{}, strict)
}

// SetFetchTimeout sets the timeout for each fetch
func SetFetchTimeout(ctx context.Context, timeout time.Duration) context.Context {
	return context.WithValue(ctx, fetchTimeoutKey{}, timeout)
}

// SetStrategyTimeout sets the timeout for each strategy
func SetStrategyTimeout(ctx context.Context, timeout time.Duration) context.Context {
	return context.WithValue(ctx, strategyTimeoutKey{}, timeout)
}

// SetStrategyTimeout sets the timeout for each strategy
func SetLogger(ctx context.Context, l log.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

// SetConfigInContext sets the config params in the context
func SetConfigInContext(ctx context.Context, s *Strategy, fallbackmode *bool, fetchTimeout, strategyTimeout *string, logger log.Logger) (context.Context, error) {
	if s != nil {
		ctx = SetStrategy(ctx, *s)
	}

	if fallbackmode != nil {
		ctx = SetStrict(ctx, !(*fallbackmode))
	}

	if fetchTimeout != nil {
		dur, err := time.ParseDuration(*fetchTimeout)
		if err != nil {
			return nil, err
		}
		ctx = SetFetchTimeout(ctx, dur)
	}

	if strategyTimeout != nil {
		dur, err := time.ParseDuration(*strategyTimeout)
		if err != nil {
			return nil, err
		}
		ctx = SetStrategyTimeout(ctx, dur)
	}

	if logger != nil {
		ctx = SetLogger(ctx, logger)
	}

	return ctx, nil
}
