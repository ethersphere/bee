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
	"sync"

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

	// stampedPut stamps a chunk against its own identity address and hands it to
	// the upload session. The identity address is the key the stamper dedups on,
	// so it has to match what the original upload used: for a content addressed
	// chunk that is the chunk address, for a single owner chunk - which every
	// dispersed replica is - it is H(socAddress || wrappedChunkAddress).
	// Getting this wrong consumes a fresh batch index for a chunk that already
	// has one, and yields a stamp the receiving reserve cannot deduplicate.
	stampedPut := func(ctx context.Context, ch swarm.Chunk) error {
		idAddr, err := storage.IdentityAddress(ch)
		if err != nil {
			return fmt.Errorf("identity address of chunk %s: %w", ch.Address(), err)
		}

		stamp, err := stamper.Stamp(ch.Address(), idAddr)
		if err != nil {
			return fmt.Errorf("stamping chunk %s: %w", ch.Address(), err)
		}

		return uploaderSession.Put(ctx, ch.WithStamp(stamp))
	}

	fn := func(addr swarm.Address) error {
		c, err := getter.Get(ctx, addr)
		if err != nil {
			return err
		}

		return stampedPut(ctx, c)
	}

	// Dispersed replicas are created per chunk trie root at upload time by the
	// hashtrie writer, not by the traversal, so traversal alone never restores
	// them. A bzz upload builds several tries - one per file plus one per
	// mantaray node - and each has replicas of its own, so every root the
	// traversal reports needs them recreated, not just the top level reference.
	var (
		replicaMu   sync.Mutex
		seenRoots   = make(map[string]struct{})
		replicaErrs []error
	)

	rootFn := func(addr swarm.Address) error {
		if rLevel == redundancy.NONE {
			return nil
		}

		// A reference can be encrypted, in which case it carries the decryption
		// key in its trailing bytes. The chunk store is keyed on the 32 byte
		// content address, so trim before looking the root chunk up.
		contentAddr := addr
		if len(addr.Bytes()) == encryption.ReferenceSize {
			contentAddr = swarm.NewAddress(addr.Bytes()[:swarm.HashSize])
		}

		replicaMu.Lock()
		_, seen := seenRoots[contentAddr.String()]
		seenRoots[contentAddr.String()] = struct{}{}
		replicaMu.Unlock()
		if seen {
			return nil
		}

		rootChunk, err := getter.Get(ctx, contentAddr)
		if err != nil {
			replicaMu.Lock()
			replicaErrs = append(replicaErrs, fmt.Errorf("get root chunk %s for dispersed replicas: %w", contentAddr, err))
			replicaMu.Unlock()
			return nil
		}

		// Only content addressed roots carry dispersed replicas. A single owner
		// chunk reference - a feed update or a GSOC - is a valid stewardship
		// target that the traversal supports, but it has no replicas to restore.
		if !cac.Valid(rootChunk) {
			return nil
		}

		// Re-pushing replicas is best effort. The content chunks are already on
		// their way, and failing the whole reupload here would throw that work
		// away and, in the API handler, skip persisting the batch indices the
		// successful pushes already consumed.
		if err := replicas.NewPutter(storage.PutterFunc(stampedPut), rLevel).Put(ctx, rootChunk); err != nil {
			replicaMu.Lock()
			replicaErrs = append(replicaErrs, fmt.Errorf("dispersed replicas of %s: %w", contentAddr, err))
			replicaMu.Unlock()
		}

		return nil
	}

	if err := s.traverser.TraverseWithRoots(ctx, root, fn, rootFn, rLevel); err != nil {
		return errors.Join(
			fmt.Errorf("traversal of %s failed: %w", root.String(), err),
			uploaderSession.Cleanup(),
		)
	}

	if err := uploaderSession.Done(root); err != nil {
		return err
	}

	// Reported only after the session is committed, so the content reupload
	// stands even when some replicas could not be pushed.
	return errors.Join(replicaErrs...)
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
