// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package getter

import (
	"context"
	"fmt"
	"time"
)

var (
	StrategyTimeout = 500 * time.Millisecond // timeout for each strategy
)

type (
	strategyKey       struct{}
	modeKey           struct{}
	fetcherTimeoutKey struct{}
	Strategy          = int
)

const (
	NONE Strategy = iota // no prefetching and no decoding
	DATA                 // just retrieve data shards no decoding
	PROX                 // proximity driven selective fetching
	RACE                 // aggressive fetching racing all chunks
	strategyCnt
)

// GetParamsFromContext extracts the strategy and strict mode from the context
func GetParamsFromContext(ctx context.Context) (s Strategy, strict bool, fetcherTimeout time.Duration) {
	s, _ = ctx.Value(strategyKey{}).(Strategy)
	strict, _ = ctx.Value(modeKey{}).(bool)
	fetcherTimeout, _ = ctx.Value(fetcherTimeoutKey{}).(time.Duration)
	return s, strict, fetcherTimeout
}

// SetFetchTimeout sets the timeout for each fetch
func SetFetchTimeout(ctx context.Context, timeout time.Duration) context.Context {
	return context.WithValue(ctx, fetcherTimeoutKey{}, timeout)
}

// SetStrategy sets the strategy for the retrieval
func SetStrategy(ctx context.Context, s Strategy) context.Context {
	return context.WithValue(ctx, strategyKey{}, s)
}

// SetStrict sets the strict mode for the retrieval
func SetStrict(ctx context.Context, strict bool) context.Context {
	return context.WithValue(ctx, modeKey{}, strict)
}

func (g *decoder) prefetch(ctx context.Context, strategy int, strict bool, strategyTimeout, fetchTimeout time.Duration) {
	if strict && strategy == NONE {
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
			timer := time.NewTimer(strategyTimeout)
			defer timer.Stop()
			stop = timer.C
		}
		lctx, cancel := context.WithTimeout(ctx, fetchTimeout)
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
	for s := strategy; s == strategy || (err != nil && !strict && s < strategyCnt); s++ {
		err = run(s)
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
