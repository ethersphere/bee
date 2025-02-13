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

	"github.com/ethersphere/bee/v2/pkg/file/joiner"
	"github.com/ethersphere/bee/v2/pkg/file/loadsave"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/manifest"
	"github.com/ethersphere/bee/v2/pkg/manifest/mantaray"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Traverser represents service which traverse through address dependent chunks.
type Traverser interface {
	// Traverse iterates through each address related to the supplied one, if possible.
	Traverse(context.Context, swarm.Address, swarm.AddressIterFunc) error
}

// New constructs for a new Traverser.
func New(getter storage.Getter, putter storage.Putter, rLevel redundancy.Level) Traverser {
	return &service{getter: getter, putter: putter, rLevel: rLevel}
}

// service is implementation of Traverser using storage.Storer as its storage.
type service struct {
	getter storage.Getter
	putter storage.Putter
	rLevel redundancy.Level
}

// Traverse implements Traverser.Traverse method.
func (s *service) Traverse(ctx context.Context, addr swarm.Address, iterFn swarm.AddressIterFunc) error {
	processBytes := func(ref swarm.Address) error {
		j, _, err := joiner.New(ctx, s.getter, s.putter, ref, s.rLevel)
		if err != nil {
			return fmt.Errorf("traversal: joiner error on %q: %w", ref, err)
		}
		err = j.IterateChunkAddresses(iterFn)
		if err != nil {
			return fmt.Errorf("traversal: iterate chunk address error for %q: %w", ref, err)
		}
		return nil
	}

	// skip SOC check for encrypted references
	if addr.IsValidLength() {
		ch, err := s.getter.Get(ctx, addr)
		if err != nil {
			return fmt.Errorf("traversal: failed to get root chunk %s: %w", addr.String(), err)
		}
		if soc.Valid(ch) {
			// if this is a SOC, the traversal will be just be the single chunk
			return iterFn(addr)
		}
	}

	j, _, err := joiner.New(ctx, s.getter, s.putter, addr, s.rLevel)
	if err != nil {
		return err
	}

	// Heuristic determination if the reference represents a manifest reference.
	// The assumption is that if the root chunk span is less than or equal to swarm.ChunkSize,
	// then the reference is likely a manifest reference. This is because manifest holds metadata
	// that points to the actual data file, and this metadata is assumed to be small - Less than or equal to swarm.ChunkSize.
	if j.Size() <= swarm.ChunkSize {
		ls := loadsave.NewReadonly(s.getter, s.putter, s.rLevel)
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
	}

	// Non-manifest processing.
	if err := processBytes(addr); err != nil {
		return fmt.Errorf("traversal: unable to process bytes for %q: %w", addr, err)
	}
	return nil
}
