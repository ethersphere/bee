// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package kvs provides functionalities needed
// for storing key-value pairs on Swarm.
//
//nolint:ireturn
package kvs

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/manifest"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var (
	// ErrNothingToSave indicates that no new key-value pair was added to the store.
	ErrNothingToSave = errors.New("nothing to save")
	// ErrNotFound is returned when an Entry is not found in the storage.
	ErrNotFound = errors.New("kvs entry not found")
)

// KeyValueStore represents a key-value store.
type KeyValueStore interface {
	// Get retrieves the value associated with the given key.
	Get(ctx context.Context, key []byte) ([]byte, error)
	// Put stores the given key-value pair in the store.
	Put(ctx context.Context, key, value []byte) error
	// Save saves key-value pair to the underlying storage and returns the reference.
	Save(ctx context.Context) (swarm.Address, error)
}

type keyValueStore struct {
	manifest manifest.Interface
	putCnt   int
}

var _ KeyValueStore = (*keyValueStore)(nil)

// Get retrieves the value associated with the given key.
func (s *keyValueStore) Get(ctx context.Context, key []byte) ([]byte, error) {
	entry, err := s.manifest.Lookup(ctx, hex.EncodeToString(key))
	if err != nil {
		switch {
		case errors.Is(err, manifest.ErrNotFound):
			return nil, ErrNotFound
		default:
			return nil, fmt.Errorf("failed to get value from manifest %w", err)
		}
	}
	ref := entry.Reference()
	return ref.Bytes(), nil
}

// Put stores the given key-value pair in the store.
func (s *keyValueStore) Put(ctx context.Context, key []byte, value []byte) error {
	err := s.manifest.Add(ctx, hex.EncodeToString(key), manifest.NewEntry(swarm.NewAddress(value), map[string]string{}))
	if err != nil {
		return fmt.Errorf("failed to put value to manifest %w", err)
	}
	s.putCnt++
	return nil
}

// Save saves key-value pair to the underlying storage and returns the reference.
func (s *keyValueStore) Save(ctx context.Context) (swarm.Address, error) {
	if s.putCnt == 0 {
		return swarm.ZeroAddress, ErrNothingToSave
	}
	ref, err := s.manifest.Store(ctx)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("failed to store manifest %w", err)
	}
	s.putCnt = 0
	return ref, nil
}

// New creates a new key-value store with a simple manifest.
func New(ls file.LoadSaver) (KeyValueStore, error) {
	m, err := manifest.NewSimpleManifest(ls)
	if err != nil {
		return nil, fmt.Errorf("failed to create simple manifest: %w", err)
	}

	return &keyValueStore{
		manifest: m,
	}, nil
}

// NewReference loads a key-value store with a simple manifest.
func NewReference(ls file.LoadSaver, ref swarm.Address) (KeyValueStore, error) {
	m, err := manifest.NewSimpleManifestReference(ref, ls)
	if err != nil {
		return nil, fmt.Errorf("failed to create simple manifest reference: %w", err)
	}

	return &keyValueStore{
		manifest: m,
	}, nil
}
