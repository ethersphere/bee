// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pinning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/traversal"
	"github.com/hashicorp/go-multierror"
)

// ErrTraversal signals that errors occurred during nodes traversal.
var ErrTraversal = errors.New("traversal iteration failed")

// Interface defines pinning operations.
type Interface interface {
	// CreatePin creates a new pin for the given reference.
	// The boolean arguments specifies whether all nodes
	// in the tree should also be traversed and pinned.
	// Repeating calls of this method are idempotent.
	CreatePin(context.Context, swarm.Address, bool) error
	// DeletePin deletes given reference. All the existing
	// nodes in the tree will also be traversed and un-pinned.
	// Repeating calls of this method are idempotent.
	DeletePin(context.Context, swarm.Address) error
	// HasPin returns true if the given reference has root pin.
	HasPin(swarm.Address) (bool, error)
	// Pins return all pinned references.
	Pins() ([]swarm.Address, error)
}

const storePrefix = "root-pin"

func rootPinKey(ref swarm.Address) string {
	return fmt.Sprintf("%s-%s", storePrefix, ref)
}

// NewService is a convenient constructor for Service.
func NewService(
	pinStorage storage.Storer,
	rhStorage storage.StateStorer,
	traverser traversal.Traverser,
) *Service {
	return &Service{
		pinStorage: pinStorage,
		rhStorage:  rhStorage,
		traverser:  traverser,
	}
}

// Service is implementation of the pinning.Interface.
type Service struct {
	pinStorage storage.Storer
	rhStorage  storage.StateStorer
	traverser  traversal.Traverser
}

// CreatePin implements Interface.CreatePin method.
func (s *Service) CreatePin(ctx context.Context, ref swarm.Address, traverse bool) error {
	// iterFn is a pinning iterator function over the leaves of the root.
	iterFn := func(leaf swarm.Address) error {
		switch err := s.pinStorage.Set(ctx, storage.ModeSetPin, leaf); {
		case errors.Is(err, storage.ErrNotFound):
			ch, err := s.pinStorage.Get(ctx, storage.ModeGetRequestPin, leaf)
			if err != nil {
				return fmt.Errorf("unable to get pin for leaf %q of root %q: %w", leaf, ref, err)
			}
			_, err = s.pinStorage.Put(ctx, storage.ModePutRequestPin, ch)
			if err != nil {
				return fmt.Errorf("unable to put pin for leaf %q of root %q: %w", leaf, ref, err)
			}
		case err != nil:
			return fmt.Errorf("unable to set pin for leaf %q of root %q: %w", leaf, ref, err)
		}
		return nil
	}

	if traverse {
		if err := s.traverser.Traverse(ctx, ref, iterFn); err != nil {
			return fmt.Errorf("traversal of %q failed: %w", ref, err)
		}
	}

	key := rootPinKey(ref)
	switch err := s.rhStorage.Get(key, new(swarm.Address)); {
	case errors.Is(err, storage.ErrNotFound):
		return s.rhStorage.Put(key, ref)
	case err != nil:
		return fmt.Errorf("unable to pin %q: %w", ref, err)
	}
	return nil
}

// DeletePin implements Interface.DeletePin method.
func (s *Service) DeletePin(ctx context.Context, ref swarm.Address) error {
	var iterErr error
	// iterFn is a unpinning iterator function over the leaves of the root.
	iterFn := func(leaf swarm.Address) error {
		err := s.pinStorage.Set(ctx, storage.ModeSetUnpin, leaf)
		if err != nil {
			iterErr = multierror.Append(err, fmt.Errorf("unable to unpin the chunk for leaf %q of root %q: %w", leaf, ref, err))
			// Continue un-pinning all chunks.
		}
		return nil
	}

	if err := s.traverser.Traverse(ctx, ref, iterFn); err != nil {
		return fmt.Errorf("traversal of %q failed: %w", ref, multierror.Append(err, iterErr))
	}
	if iterErr != nil {
		return multierror.Append(ErrTraversal, iterErr)
	}

	key := rootPinKey(ref)
	if err := s.rhStorage.Delete(key); err != nil {
		return fmt.Errorf("unable to delete pin for key %q: %w", key, err)
	}
	return nil
}

// HasPin implements Interface.HasPin method.
func (s *Service) HasPin(ref swarm.Address) (bool, error) {
	key, val := rootPinKey(ref), swarm.NewAddress(nil)
	switch err := s.rhStorage.Get(key, &val); {
	case errors.Is(err, storage.ErrNotFound):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("unable to get pin for key %q: %w", key, err)
	}
	return val.Equal(ref), nil
}

// Pins implements Interface.Pins method.
func (s *Service) Pins() ([]swarm.Address, error) {
	var refs []swarm.Address
	err := s.rhStorage.Iterate(storePrefix, func(key, val []byte) (stop bool, err error) {
		var ref swarm.Address
		if err := json.Unmarshal(val, &ref); err != nil {
			return true, fmt.Errorf("invalid reference value %q: %w", string(val), err)
		}
		refs = append(refs, ref)
		return false, nil
	})
	if err != nil {
		return nil, fmt.Errorf("iteration failed: %w", err)
	}
	return refs, nil
}
