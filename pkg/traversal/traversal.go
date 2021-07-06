// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package traversal provides abstraction and implementation
// needed to traverse all chunks below a given root hash.
// It tries to parse all manifests and collections in its
// attempt to log all chunk addresses on the way.
package traversal

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/file/loadsave"
	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/manifest/mantaray"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Traverser represents service which traverse through address dependent chunks.
type Traverser interface {
	// Traverse iterates through each address related to the supplied one, if possible.
	Traverse(context.Context, swarm.Address, swarm.AddressIterFunc) error
}

type PutGetter interface {
	storage.Putter
	storage.Getter
}

// New constructs for a new Traverser.
func New(store PutGetter) Traverser {
	return &service{store: store}
}

// service is implementation of Traverser using storage.Storer as its storage.
type service struct {
	store PutGetter
}

// Traverse implements Traverser.Traverse method.
func (s *service) Traverse(ctx context.Context, addr swarm.Address, iterFn swarm.AddressIterFunc) error {
	processBytes := func(ref swarm.Address) error {
		j, _, err := joiner.New(ctx, s.store, ref)
		if err != nil {
			return fmt.Errorf("traversal: joiner error on %q: %w", ref, err)
		}
		err = j.IterateChunkAddresses(iterFn)
		if err != nil {
			return fmt.Errorf("traversal: iterate chunk address error for %q: %w", ref, err)
		}
		return nil
	}

	ls := loadsave.New(s.store, storage.ModePutRequest, false)
	switch mf, err := manifest.NewDefaultManifestReference(addr, ls); {
	case errors.Is(err, manifest.ErrInvalidManifestType):
		break
	case err != nil:
		return fmt.Errorf("traversal: unable to create manifest reference for %q: %w", addr, err)
	default:
		err := mf.IterateAddresses(ctx, processBytes)
		if errors.Is(err, mantaray.ErrTooShort) || errors.Is(err, mantaray.ErrInvalidVersionHash) {
			// Based on the returned errors we conclude that it might
			// not be a manifest, so we try non-manifest processing.
			break
		}
		if err != nil {
			return fmt.Errorf("traversal: unable to process bytes for %q: %w", addr, err)
		}
		return nil
	}

	// Non-manifest processing.
	if err := processBytes(addr); err != nil {
		return fmt.Errorf("traversal: unable to process bytes for %q: %w", addr, err)
	}
	return nil
}
