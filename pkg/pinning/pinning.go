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
	// CreatePin creates a new pin for the given address.
	// The boolean arguments specifies whether all nodes
	// in the tree should also be traversed and pinned.
	// Repeating calls of this method are idempotent.
	CreatePin(context.Context, swarm.Address, bool) error
	// DeletePin deletes given address. All the existing
	// nodes in the tree will also be traversed and un-pinned.
	// Repeating calls of this method are idempotent.
	DeletePin(context.Context, swarm.Address) error
	// HasPin returns true if the given address has root pin.
	HasPin(swarm.Address) (bool, error)
	// Pins return all pinned addresses.
	Pins() ([]swarm.Address, error)
}

const storePrefix = "root-pin"

func rootPinKey(addr swarm.Address) string {
	return fmt.Sprintf("%s-%s", storePrefix, addr)
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
func (s *Service) CreatePin(ctx context.Context, addr swarm.Address, traverse bool) error {
	// iterFn is a pinning iterator function over the leaves of the root.
	iterFn := func(leaf swarm.Address) error {
		switch err := s.pinStorage.Set(ctx, storage.ModeSetPin, leaf); {
		case errors.Is(err, storage.ErrNotFound):
			ch, err := s.pinStorage.Get(ctx, storage.ModeGetRequestPin, leaf)
			if err != nil {
				return fmt.Errorf("unable to get pin for leaf %q of root %q: %w", leaf, addr, err)
			}
			_, err = s.pinStorage.Put(ctx, storage.ModePutRequestPin, ch)
			if err != nil {
				return fmt.Errorf("unable to put pin for leaf %q of root %q: %w", leaf, addr, err)
			}
		case err != nil:
			return fmt.Errorf("unable to set pin for leaf %q of root %q: %w", leaf, addr, err)
		}
		return nil
	}

	if traverse {
		if err := s.traverser.Traverse(ctx, addr, iterFn); err != nil {
			return fmt.Errorf("traversal of %q failed: %w", addr, err)
		}
	}

	key := rootPinKey(addr)
	switch err := s.rhStorage.Get(key, new(swarm.Address)); {
	case errors.Is(err, storage.ErrNotFound):
		return s.rhStorage.Put(key, addr)
	case err != nil:
		return fmt.Errorf("unable to pin %q: %w", addr, err)
	}
	return nil
}

// DeletePin implements Interface.DeletePin method.
func (s *Service) DeletePin(ctx context.Context, addr swarm.Address) error {
	var iterErr error
	// iterFn is a unpinning iterator function over the leaves of the root.
	iterFn := func(leaf swarm.Address) error {
		err := s.pinStorage.Set(ctx, storage.ModeSetUnpin, leaf)
		if err != nil {
			iterErr = multierror.Append(err, fmt.Errorf("unable to unpin the chunk for leaf %q of root %q: %w", leaf, addr, err))
			// Continue un-pinning all chunks.
		}
		return nil
	}

	if err := s.traverser.Traverse(ctx, addr, iterFn); err != nil {
		return fmt.Errorf("traversal of %q failed: %w", addr, multierror.Append(err, iterErr))
	}
	if iterErr != nil {
		return multierror.Append(ErrTraversal, iterErr)
	}

	key := rootPinKey(addr)
	if err := s.rhStorage.Delete(key); err != nil {
		return fmt.Errorf("unable to delete pin for key %q: %w", key, err)
	}
	return nil
}

// HasPin implements Interface.HasPin method.
func (s *Service) HasPin(addr swarm.Address) (bool, error) {
	key, val := rootPinKey(addr), swarm.NewAddress(nil)
	switch err := s.rhStorage.Get(key, &val); {
	case errors.Is(err, storage.ErrNotFound):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("unable to get pin for key %q: %w", key, err)
	}
	return val.Equal(addr), nil
}

// Pins implements Interface.Pins method.
func (s *Service) Pins() ([]swarm.Address, error) {
	var addrs []swarm.Address
	err := s.rhStorage.Iterate(storePrefix, func(key, val []byte) (stop bool, err error) {
		var addr swarm.Address
		if err := json.Unmarshal(val, &addr); err != nil {
			return true, fmt.Errorf("invalid address value %q: %w", string(val), err)
		}
		addrs = append(addrs, addr)
		return false, nil
	})
	if err != nil {
		return nil, fmt.Errorf("iteration failed: %w", err)
	}
	return addrs, nil
}
