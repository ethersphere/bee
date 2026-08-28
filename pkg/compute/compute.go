// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compute

import (
	"context"
	"errors"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"golang.org/x/sync/semaphore"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "compute"

// ErrBusy is returned when all execution workers are occupied.
var ErrBusy = errors.New("compute: all workers busy")

// Options configures the compute Service.
type Options struct {
	// Workers bounds the number of concurrent executions. Values < 1 become 1.
	Workers int
	// Watchdog is a wall-clock safety timeout that kills a hung execution. It is
	// NOT a deterministic budget and a kill yields StatusHostError.
	Watchdog time.Duration
	// Logger is used for operator diagnostics.
	Logger log.Logger
}

// Service is the node-facing execution service. It bounds concurrency and
// applies the watchdog around an Engine.
//
// Phase 0 delegates to an in-process wazero engine; the Engine boundary lets a
// later phase swap in the deterministic out-of-process worker without touching
// callers.
type Service struct {
	engine   Engine
	sem      *semaphore.Weighted
	watchdog time.Duration
	logger   log.Logger
}

// New constructs a compute Service.
func New(o Options) (*Service, error) {
	if o.Workers < 1 {
		o.Workers = 1
	}
	logger := o.Logger
	if logger == nil {
		logger = log.Noop
	}
	logger = logger.WithName(loggerName).Register()

	logger.Warning("wasm execute is experimental: the phase-0 engine does not enforce deterministic gas and its output is not reproducible across nodes")

	return &Service{
		engine:   newWazeroEngine(logger),
		sem:      semaphore.NewWeighted(int64(o.Workers)),
		watchdog: o.Watchdog,
		logger:   logger,
	}, nil
}

// Execute runs a module, bounding concurrency and applying the watchdog timeout.
// It returns ErrBusy without blocking when no worker slot is free.
func (s *Service) Execute(ctx context.Context, req Request) (Result, error) {
	if !s.sem.TryAcquire(1) {
		return Result{}, ErrBusy
	}
	defer s.sem.Release(1)

	if s.watchdog > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.watchdog)
		defer cancel()
	}

	return s.engine.Execute(ctx, req)
}

// Close releases engine resources.
func (s *Service) Close() error {
	return s.engine.Close()
}
