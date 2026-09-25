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

	if err := s.traverser.Traverse(ctx, root, fn, rLevel); err != nil {
		return errors.Join(
			fmt.Errorf("traversal of %s failed: %w", root.String(), err),
			uploaderSession.Cleanup(),
		)
	}

	if rLevel != redundancy.NONE {
		// Dispersed replicas are keyed on the 32-byte content address. root can be
		// an encrypted reference (address + decryption key), so trim it before
		// deriving replica addresses, or they won't match what a downloader
		// deriving replicas from the plain address expects.
		contentAddr := root
		if len(root.Bytes()) == encryption.ReferenceSize {
			contentAddr = swarm.NewAddress(root.Bytes()[:swarm.HashSize])
		}

		rootChunk, err := getter.Get(ctx, contentAddr)
		if err != nil {
			return errors.Join(fmt.Errorf("get root chunk for dispersed replicas: %w", err), uploaderSession.Cleanup())
		}

		if !cac.Valid(rootChunk) {
			return errors.Join(fmt.Errorf("root chunk %s is not a valid content-addressed chunk", contentAddr), uploaderSession.Cleanup())
		}

		// Stamp each replica individually as it is put, keyed on its own SOC
		// address - not the root chunk's address, which replicas.NewPutter
		// wraps into a differently-addressed SOC chunk per replica.
		stampedPutter := storage.PutterFunc(func(ctx context.Context, ch swarm.Chunk) error {
			stamp, err := stamper.Stamp(ch.Address(), ch.Address())
			if err != nil {
				return fmt.Errorf("stamping replica %s: %w", ch.Address(), err)
			}
			return uploaderSession.Put(ctx, ch.WithStamp(stamp))
		})

		if err := replicas.NewPutter(stampedPutter, rLevel).Put(ctx, rootChunk); err != nil {
			return errors.Join(fmt.Errorf("re-uploading dispersed replicas: %w", err), uploaderSession.Cleanup())
		}
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
