// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package stewardess provides convenience methods
// for reseeding content on Swarm.
package steward

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/pushsync"
	"github.com/ethersphere/bee/pkg/retrieval"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/traversal"
	"golang.org/x/sync/errgroup"
)

// how many parallel push operations
const parallelPush = 5

type Interface interface {
	// Reupload root hash and all of its underlying
	// associated chunks to the network.
	Reupload(context.Context, swarm.Address) error

	// IsRetrievable checks whether the content
	// on the given address is retrievable.
	IsRetrievable(context.Context, swarm.Address) (bool, error)
}

type steward struct {
	getter    storage.Getter
	push      pushsync.PushSyncer
	traverser traversal.Traverser
	retrieval retrieval.Interface
}

func New(getter storage.Getter, t traversal.Traverser, r retrieval.Interface, p pushsync.PushSyncer) Interface {
	return &steward{getter: getter, push: p, traverser: t, retrieval: r}
}

// Reupload content with the given root hash to the network.
// The service will automatically dereference and traverse all
// addresses and push every chunk individually to the network.
// It assumes all chunks are available locally. It is therefore
// advisable to pin the content locally before trying to reupload it.
func (s *steward) Reupload(ctx context.Context, root swarm.Address) error {
	sem := make(chan struct{}, parallelPush)
	eg, _ := errgroup.WithContext(ctx)
	fn := func(addr swarm.Address) error {
		c, err := s.getter.Get(ctx, storage.ModeGetSync, addr)
		if err != nil {
			return err
		}

		sem <- struct{}{}
		eg.Go(func() error {
			defer func() { <-sem }()
			_, err := s.push.PushChunkToClosest(ctx, c)
			if err != nil {
				if !errors.Is(err, topology.ErrWantSelf) {
					return err
				}
				// swallow the error in case we are the closest node
			}
			return nil
		})
		return nil
	}

	if err := s.traverser.Traverse(ctx, root, fn); err != nil {
		return fmt.Errorf("traversal of %s failed: %w", root.String(), err)
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("push error during reupload: %w", err)
	}
	return nil
}

// IsRetrievable implements Interface.IsRetrievable method.
func (s *steward) IsRetrievable(ctx context.Context, root swarm.Address) (bool, error) {
	iterFn := func(leaf swarm.Address) error {
		_, err := s.retrieval.RetrieveChunk(ctx, leaf, true)
		return err
	}
	switch err := s.traverser.Traverse(ctx, root, iterFn); {
	case errors.Is(err, storage.ErrNotFound):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("traversal of %q failed: %w", root, err)
	default:
		return true, nil
	}
}
