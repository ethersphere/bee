// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.package storage

// Package stewardess provides convenience methods
// for reseeding content on Swarm.
package steward

import (
	"context"
	"fmt"

	"github.com/ethersphere/bee/pkg/pushsync"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/traversal"
	"golang.org/x/sync/errgroup"
)

// how many parallel push operations
const parallelPush = 5

type Reuploader interface {
	// Reupload root hash and all of its underlying
	// associated chunks to the network.
	Reupload(context.Context, swarm.Address) error
}

type steward struct {
	getter    storage.Getter
	push      pushsync.PushSyncer
	traverser traversal.Traverser
}

func New(getter storage.Getter, t traversal.Traverser, p pushsync.PushSyncer) Reuploader {
	return &steward{getter: getter, push: p, traverser: t}
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
				return err
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
