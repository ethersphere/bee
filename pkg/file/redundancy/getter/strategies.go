// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package getter

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/ethersphere/bee/pkg/retrieval"
)

const (
	DefaultStrategy        = NONE                           // default prefetching strategy
	DefaultStrict          = true                           // default fallback modes
	DefaultFetchTimeout    = retrieval.RetrieveChunkTimeout // timeout for each chunk retrieval
	DefaultStrategyTimeout = 300 * time.Millisecond         // timeout for each strategy
)

var (
	ErrNotEnoughShards     = errors.New("not enough shards to reconstruct")
	ErrRecoveryUnavailable = errors.New("recovery disabled for strategy")
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

// SetConfigInContext sets the config params in the context
func SetConfigInContext(ctx context.Context, s *Strategy, fallbackmode *bool, fetchTimeout, strategyTimeout *string) (context.Context, error) {
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

	return ctx, nil
}

func (d *decoder) run(ctx context.Context) {
	// prefetch chunks according to strategy
	var strategies []Strategy
	if d.config.Strategy > NONE || !d.config.Strict {
		strategies = []Strategy{d.config.Strategy}
	}
	if !d.config.Strict {
		for i := d.config.Strategy + 1; i < strategyCnt; i++ {
			strategies = append(strategies, i)
		}
	}
	d.prefetch(ctx, strategies)
	d.cancel()
	d.wg.Done()
	d.remove()
}

func (g *decoder) prefetch(ctx context.Context, strategies []Strategy) {
	// context to cancel when shardCnt chunks are retrieved
	lctx, cancel := context.WithCancel(ctx)
	defer cancel()

	run := func(s Strategy) (err error) {
		if s == PROX { // NOT IMPLEMENTED
			return nil
		}

		var timeout <-chan time.Time
		if s < RACE {
			timer := time.NewTimer(g.config.StrategyTimeout)
			defer timer.Stop()
			timeout = timer.C
		}
		prefetch(lctx, g, s)

		select {
		case <-g.fetched.c:
			// successfully retrieved shardCnt number of chunks
			g.fetched.cancel()
			g.failed.cancel()
			cancel()
			// sdignal that decoding is finished
			return g.recover(ctx)

		case <-timeout: // strategy timeout
			return nil

		case <-ctx.Done(): // context cancelled
			return ctx.Err()
		}
	}
	for _, s := range strategies {
		if err := run(s); err != nil {
			break
		}
	}
}

// prefetch launches the retrieval of chunks based on the strategy
func prefetch(ctx context.Context, g *decoder, s Strategy) {
	switch s {
	case NONE:
		return
	case DATA:
		// only retrieve data shards
		for i := range g.waits {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if g.fly(i) {
				i := i
				g.wg.Add(1)
				go g.fetch(ctx, i)
			}
		}
	case PROX:
		// proximity driven selective fetching
		// NOT IMPLEMENTED
	case RACE:
		// retrieve all chunks at once enabling race among chunks
		// only retrieve data shards
		for i := range g.addrs {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if i >= g.shardCnt || g.fly(i) {
				i := i
				g.wg.Add(1)
				go g.fetch(ctx, i)
			}
		}
	}
}

// counter counts the number of successful or failed retrievals
// count counts the number of successful or failed retrievals
// if either successful retrievals reach shardCnt or failed ones reach parityCnt + 1,
// it signals by true or false respectively on the channel argument and terminates
type counter struct {
	c   chan struct{}
	n   atomic.Int32
	max int
	off atomic.Bool
}

func newCounter(max int) *counter {
	return &counter{
		c:   make(chan struct{}),
		max: max,
	}
}

func (c *counter) inc() {
	if !c.off.Load() && c.n.Add(1) == int32(c.max)+1 {
		close(c.c)
	}
}

func (c *counter) cancel() {
	c.off.Store(true)
}
