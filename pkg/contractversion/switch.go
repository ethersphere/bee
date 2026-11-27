// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package contractversion provides a generic switch that selects between
// contract implementations according to the current block height.
package contractversion

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync/atomic"

	"github.com/ethersphere/bee/v2/pkg/log"
)

// ErrInvalidVersions is returned when the version list is empty, does not
// start at block 0 or is not strictly increasing by FromBlock.
var ErrInvalidVersions = errors.New("contractversion: invalid versions")

// BlockNumberer provides the current block number.
type BlockNumberer interface {
	BlockNumber(ctx context.Context) (uint64, error)
}

// Version is an implementation active from FromBlock (inclusive).
type Version[T any] struct {
	FromBlock uint64
	Impl      T
}

// Switch selects an implementation based on block height.
type Switch[T any] struct {
	logger   log.Logger
	name     string
	backend  BlockNumberer
	versions []Version[T]
	last     atomic.Int64
}

// New creates a Switch. versions must be non-empty, start at block 0 and be
// strictly increasing by FromBlock.
func New[T any](logger log.Logger, name string, backend BlockNumberer, versions []Version[T]) (*Switch[T], error) {
	if len(versions) == 0 {
		return nil, fmt.Errorf("%w: no versions for %s", ErrInvalidVersions, name)
	}
	if versions[0].FromBlock != 0 {
		return nil, fmt.Errorf("%w: first version of %s must start at block 0", ErrInvalidVersions, name)
	}
	for i := 1; i < len(versions); i++ {
		if versions[i].FromBlock <= versions[i-1].FromBlock {
			return nil, fmt.Errorf("%w: versions of %s not strictly increasing", ErrInvalidVersions, name)
		}
	}
	s := &Switch[T]{
		logger:   logger.WithName("contractversion").Register(),
		name:     name,
		backend:  backend,
		versions: append([]Version[T](nil), versions...),
	}
	s.last.Store(-1)
	return s, nil
}

func (s *Switch[T]) index(block uint64) int {
	return sort.Search(len(s.versions), func(i int) bool {
		return s.versions[i].FromBlock > block
	}) - 1
}

// At returns the implementation with the highest FromBlock <= block.
func (s *Switch[T]) At(block uint64) T {
	return s.versions[s.index(block)].Impl
}

// Active returns the implementation for the backend's current block number.
func (s *Switch[T]) Active(ctx context.Context) (T, error) {
	block, err := s.backend.BlockNumber(ctx)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("block number: %w", err)
	}
	i := s.index(block)
	if prev := s.last.Swap(int64(i)); prev != -1 && prev != int64(i) {
		s.logger.Info("contract version switched", "contract", s.name, "from_block", s.versions[i].FromBlock)
	}
	return s.versions[i].Impl, nil
}
