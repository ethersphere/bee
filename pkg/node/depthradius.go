// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/depth"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storer"
)

// newInitialRadiusFunc returns the radius function the reserve worker calls on
// a fresh full node. A known depth gives the depth minus the doubling, no
// history gives the minimum. While there is no data it waits on the provider
// until two round lengths after its first call, then, once batch sync is done,
// falls back to the batchstore ceiling minus the doubling. A commitment read
// error is logged and both the provider and the commitment are tried again one
// round length later. The result is never below the minimum.
func newInitialRadiusFunc(p depth.Provider, doubling, minimum uint8, roundLength time.Duration, commitment func() (uint64, error), syncDone func() bool, logger log.Logger) func(context.Context) (uint8, error) {
	var (
		once     sync.Once
		deadline time.Time
	)

	return func(ctx context.Context) (uint8, error) {
		once.Do(func() { deadline = time.Now().Add(2 * roundLength) })

		timeout := time.Until(deadline)
		for {
			d, s, err := waitDepth(ctx, p, timeout)
			if err != nil {
				return 0, err
			}

			switch s {
			case depth.StateKnown:
				r := max(subClamped(d, doubling), minimum)
				logger.Info("initial storage radius", "reason", "known", "radius", r, "network_depth", d)
				return r, nil
			case depth.StateNoHistory:
				logger.Info("initial storage radius", "reason", "no_history", "radius", minimum)
				return minimum, nil
			}

			timeout = roundLength
			if !syncDone() {
				continue
			}

			c, err := commitment()
			if err != nil {
				logger.Warning("batchstore commitment for the initial radius fallback", "error", err)
				continue
			}
			ceiling := depth.Ceiling(c, storer.DefaultReserveCapacity)
			r := max(subClamped(ceiling, doubling), minimum)
			logger.Warning("network depth unknown, initial storage radius from the batchstore ceiling", "reason", "depth_unknown_fallback", "ceiling", ceiling, "radius", r)
			return r, nil
		}
	}
}

// newLightRadiusFunc returns the radius function of a node without a reserve
// worker. A known depth is returned as is. While there is no data it waits on
// the provider until one round length after construction; after that, and in
// no history, it returns depth.ErrUnknown at once.
func newLightRadiusFunc(p depth.Provider, roundLength time.Duration, now func() time.Time) func(context.Context) (uint8, error) {
	deadline := now().Add(roundLength)

	return func(ctx context.Context) (uint8, error) {
		d, s := p.Depth()
		if s == depth.StateNoData {
			timeout := deadline.Sub(now())
			if timeout <= 0 {
				return 0, depth.ErrUnknown
			}
			var err error
			if d, s, err = waitDepth(ctx, p, timeout); err != nil {
				return 0, err
			}
		}
		if s != depth.StateKnown {
			return 0, depth.ErrUnknown
		}
		return d, nil
	}
}

// newFullRadiusFunc returns the radius function of a full node with a reserve
// worker: it waits until the worker has set the first radius, then returns the
// local storage radius, never below the minimum.
func newFullRadiusFunc(ready <-chan struct{}, local func() uint8, minimum uint8) func(context.Context) (uint8, error) {
	return func(ctx context.Context) (uint8, error) {
		select {
		case <-ready:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
		return max(local(), minimum), nil
	}
}

// waitDepth waits on p for at most timeout while the state is no data. It
// returns the state at the end of the wait with no error when the timeout
// passes, and the context error when ctx is done.
func waitDepth(ctx context.Context, p depth.Provider, timeout time.Duration) (uint8, depth.State, error) {
	if d, s := p.Depth(); s != depth.StateNoData || timeout <= 0 {
		return d, s, nil
	}

	wctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	d, s, err := p.Wait(wctx)
	switch {
	case err == nil:
		return d, s, nil
	case ctx.Err() != nil:
		return 0, 0, ctx.Err()
	case errors.Is(err, context.DeadlineExceeded):
		return d, s, nil
	default:
		return 0, 0, fmt.Errorf("network depth: %w", err)
	}
}

// subClamped returns a - b, or 0 when b is larger.
func subClamped(a, b uint8) uint8 {
	if a < b {
		return 0
	}
	return a - b
}
