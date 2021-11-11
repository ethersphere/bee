// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pinning

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/traversal"
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
	contextStore storage.ContextStorer,
) *Service {
	return &Service{
		pinStorage:   pinStorage,
		rhStorage:    rhStorage,
		traverser:    traverser,
		contextStore: contextStore,
	}
}

// Service is implementation of the pinning.Interface.
type Service struct {
	pinStorage   storage.Storer
	rhStorage    storage.StateStorer
	traverser    traversal.Traverser
	contextStore storage.ContextStorer
}

// CreatePin implements Interface.CreatePin method.
func (s *Service) CreatePin(ctx context.Context, ref swarm.Address, traverse bool) error {
	// iterFn is a pinning iterator function over the leaves of the root.
	pinCtx := s.contextStore.PinContext()
	pinStore, cleanup, err := pinCtx.GetByName(ref.String())
	defer cleanup()
	if err != nil {
		return fmt.Errorf("get pin store by name: %w", err)
	}

	lookupCtx := s.contextStore.LookupContext()
	iterFn := func(leaf swarm.Address) error {
		ch, err := lookupCtx.Get(ctx, leaf)
		if err != nil {
			return fmt.Errorf("unable to get pin for leaf %q of root %q: %w", leaf, ref, err)
		}
		_, err = pinStore.Put(ctx, ch)
		if err != nil {
			return fmt.Errorf("unable to put pin for leaf %q of root %q: %w", leaf, ref, err)
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
	return s.contextStore.PinContext().Unpin(ref)
}

// HasPin implements Interface.HasPin method.
func (s *Service) HasPin(ref swarm.Address) (bool, error) {
	pins, err := s.contextStore.PinContext().Pins()
	if err != nil {
		return false, err
	}

	for _, val := range pins {
		if val.Equal(ref) {
			return true, nil
		}
	}
	return false, nil
}

// Pins implements Interface.Pins method.
func (s *Service) Pins() ([]swarm.Address, error) {
	return s.contextStore.PinContext().Pins()
}
