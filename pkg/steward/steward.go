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

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/encryption"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/replicas"
	"github.com/ethersphere/bee/v2/pkg/retrieval"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/traversal"
)

type Interface interface {
	// Reupload root hash and all of its underlying
	// associated chunks to the network.
	Reupload(context.Context, swarm.Address, postage.Stamper, redundancy.Level) error

	// IsRetrievable checks whether the content
	// on the given address is retrievable.
	IsRetrievable(context.Context, swarm.Address, redundancy.Level) (bool, error)
}

type steward struct {
	netStore     storer.NetStore
	traverser    traversal.Traverser
	netTraverser traversal.Traverser
	netGetter    retrieval.Interface
}

func New(ns storer.NetStore, r retrieval.Interface, joinerPutter storage.Putter) Interface {
	return &steward{
		netStore:     ns,
		traverser:    traversal.New(ns.Download(true), joinerPutter),
		netTraverser: traversal.New(&netGetter{r}, joinerPutter),
		netGetter:    r,
	}
}

// Reupload content with the given root hash to the network.
// The service will automatically dereference and traverse all
// addresses and push every chunk individually to the network.
// It assumes all chunks are available locally. It is therefore
// advisable to pin the content locally before trying to reupload it.
func (s *steward) Reupload(ctx context.Context, root swarm.Address, stamper postage.Stamper, rLevel redundancy.Level) error {
	uploaderSession := s.netStore.DirectUpload()
	getter := s.netStore.Download(false)

	fn := func(addr swarm.Address) error {
		c, err := getter.Get(ctx, addr)
		if err != nil {
			return err
		}

		stamp, err := stamper.Stamp(c.Address(), c.Address())
		if err != nil {
			return fmt.Errorf("stamping chunk %s: %w", c.Address(), err)
		}

		return uploaderSession.Put(ctx, c.WithStamp(stamp))
	}

	// Dispersed replicas exist for every joiner root (the reference and, for
	// manifests, every node and entry), so re-create them for each root.
	var opts []traversal.Option
	if rLevel != redundancy.NONE {
		seen := make(map[string]struct{})
		replicaPutter := replicas.NewPutter(storage.PutterFunc(func(ctx context.Context, ch swarm.Chunk) error {
			idAddress, err := storage.IdentityAddress(ch)
			if err != nil {
				return fmt.Errorf("identity address for replica %s: %w", ch.Address(), err)
			}
			stamp, err := stamper.Stamp(ch.Address(), idAddress)
			if err != nil {
				return fmt.Errorf("stamping replica %s: %w", ch.Address(), err)
			}
			return uploaderSession.Put(ctx, ch.WithStamp(stamp))
		}), rLevel)

		opts = append(opts, traversal.WithRootFn(func(ref swarm.Address) error {
			// replicas are keyed on the 32-byte content address, so trim encrypted references
			addr := ref
			if len(ref.Bytes()) == encryption.ReferenceSize {
				addr = swarm.NewAddress(ref.Bytes()[:swarm.HashSize])
			}
			if _, ok := seen[addr.ByteString()]; ok {
				return nil
			}
			seen[addr.ByteString()] = struct{}{}

			rootChunk, err := getter.Get(ctx, addr)
			if err != nil {
				return fmt.Errorf("get root chunk %s for dispersed replicas: %w", addr, err)
			}
			// replicas only exist for content-addressed roots
			if !cac.Valid(rootChunk) {
				return nil
			}
			if err := replicaPutter.Put(ctx, rootChunk); err != nil {
				return fmt.Errorf("re-uploading dispersed replicas of %s: %w", addr, err)
			}
			return nil
		}))
	}

	if err := s.traverser.Traverse(ctx, root, fn, rLevel, opts...); err != nil {
		return errors.Join(
			fmt.Errorf("traversal of %s failed: %w", root.String(), err),
			uploaderSession.Cleanup(),
		)
	}

	return uploaderSession.Done(root)
}

// IsRetrievable implements Interface.IsRetrievable method.
func (s *steward) IsRetrievable(ctx context.Context, root swarm.Address, rLevel redundancy.Level) (bool, error) {
	fn := func(a swarm.Address) error {
		_, err := s.netGetter.RetrieveChunk(ctx, a, swarm.ZeroAddress)
		return err
	}
	switch err := s.netTraverser.Traverse(ctx, root, fn, rLevel); {
	case errors.Is(err, storage.ErrNotFound):
		return false, nil
	case errors.Is(err, topology.ErrNotFound):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("traversal of %q failed: %w", root, err)
	default:
		return true, nil
	}
}

// netGetter implements the storage Getter.Get method in a way
// that it will try to retrieve the chunk only from the network.
type netGetter struct {
	retrieval retrieval.Interface
}

// Get implements the storage Getter.Get interface.
func (ng *netGetter) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	return ng.retrieval.RetrieveChunk(ctx, addr, swarm.ZeroAddress)
}
