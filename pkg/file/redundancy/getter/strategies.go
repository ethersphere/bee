// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package getter

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	DefaultStrategy        = NONE                   // default prefetching strategy
	DefaultStrict          = true                   // default fallback modes
	DefaultFetchTimeout    = 30 * time.Second       // timeout for each chunk retrieval
	DefaultStrategyTimeout = 300 * time.Millisecond // timeout for each strategy
)

type (
	strategyKey        struct{}
	modeKey            struct{}
	fetchTimeoutKey    struct{}
	strategyTimeoutKey struct{}
	Strategy           = int
)

// Config is the configuration for the getter - public
type Config struct {
	Strategy        Strategy
	Strict          bool
	FetchTimeout    time.Duration
	StrategyTimeout time.Duration
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
		fetchTimeoutVal, ok := val.(string)
		if !ok {
			return conf, e("fetcher timeout")
		}
		conf.FetchTimeout, err = time.ParseDuration(fetchTimeoutVal)
		if err != nil {
			return conf, e("fetcher timeout", err)
		}
	}
	if val := ctx.Value(strategyTimeoutKey{}); val != nil {
		strategyTimeoutVal, ok := val.(string)
		if !ok {
			return conf, e("fetcher timeout")
		}
		conf.StrategyTimeout, err = time.ParseDuration(strategyTimeoutVal)
		if err != nil {
			return conf, e("fetcher timeout", err)
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
func SetFetchTimeout(ctx context.Context, timeout string) context.Context {
	return context.WithValue(ctx, fetchTimeoutKey{}, timeout)
}

// SetStrategyTimeout sets the timeout for each strategy
func SetStrategyTimeout(ctx context.Context, timeout string) context.Context {
	return context.WithValue(ctx, fetchTimeoutKey{}, timeout)
}

// SetConfigInContext sets the config params in the context
func SetConfigInContext(ctx context.Context, s Strategy, fallbackmode bool, fetchTimeout, strategyTimeout string) context.Context {
	ctx = SetStrategy(ctx, s)
	ctx = SetStrict(ctx, !fallbackmode)
	ctx = SetFetchTimeout(ctx, fetchTimeout)
	ctx = SetStrategyTimeout(ctx, strategyTimeout)
	return ctx
}

func (g *decoder) prefetch(ctx context.Context) {
	if g.config.Strict && g.config.Strategy == NONE {
		return
	}
	defer g.remove()
	var cancels []func()
	cancelAll := func() {
		for _, cancel := range cancels {
			cancel()
		}
	}
	defer cancelAll()
	run := func(s Strategy) error {
		if s == PROX { // NOT IMPLEMENTED
			return fmt.Errorf("strategy %d not implemented", s)
		}

		var stop <-chan time.Time
		if s < RACE {
			timer := time.NewTimer(g.config.StrategyTimeout)
			defer timer.Stop()
			stop = timer.C
		}
		lctx, cancel := context.WithCancel(ctx)
		cancels = append(cancels, cancel)
		prefetch(lctx, g, s)

		select {
		// successfully retrieved shardCnt number of chunks
		case <-g.ready:
			cancelAll()
		case <-stop:
			return fmt.Errorf("prefetching with strategy %d timed out", s)
		case <-ctx.Done():
			return nil
		}
		// call the erasure decoder
		// if decoding is successful terminate the prefetch loop
		return g.recover(ctx) // context to cancel when shardCnt chunks are retrieved
	}
	var err error
	for s := g.config.Strategy; s < strategyCnt; s++ {
		err = run(s)
		if g.config.Strict || err == nil {
			break
		}
	}

	if err != nil {
		g.err = err
	}
}

// prefetch launches the retrieval of chunks based on the strategy
func prefetch(ctx context.Context, g *decoder, s Strategy) {
	var m []int
	switch s {
	case NONE:
		return
	case DATA:
		// only retrieve data shards
		m = g.missing()
	case PROX:
		// proximity driven selective fetching
		// NOT IMPLEMENTED
	case RACE:
		// retrieve all chunks at once enabling race among chunks
		m = g.missing()
		for i := g.shardCnt; i < len(g.addrs); i++ {
			m = append(m, i)
		}
	}
	for _, i := range m {
		i := i
		g.wg.Add(1)
		go func() {
			g.fetch(ctx, i)
			g.wg.Done()
		}()
	}
}
