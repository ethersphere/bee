// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package manifest

import (
	"context"
	"errors"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const DefaultManifestType = ManifestSimpleContentType

var (
	// ErrNotFound is returned when an Entry is not found in the manifest.
	ErrNotFound = errors.New("manifest: not found")

	// ErrInvalidManifestType is returned when an unknown manifest type
	// is provided to the function.
	ErrInvalidManifestType = errors.New("manifest: invalid type")
)

// Interface for operations with manifest.
type Interface interface {
	// Type returns manifest implementation type information
	Type() string
	// Add a manifest entry to the specified path.
	Add(string, Entry) error
	// Remove a manifest entry on the specified path.
	Remove(string) error
	// Lookup returns a manifest entry if one is found in the specified path.
	Lookup(string) (Entry, error)
	// Store stores the manifest, returning the resulting address.
	Store(storage.ModePut) (swarm.Address, error)
}

// Entry represents a single manifest entry.
type Entry interface {
	// Reference returns the address of the file.
	Reference() swarm.Address
}

// NewDefaultManifest creates a new manifest with default type.
func NewDefaultManifest(
	ctx context.Context,
	encrypted bool,
	storer storage.Storer,
) (Interface, error) {
	return NewManifest(ctx, DefaultManifestType, encrypted, storer)
}

// NewManifest creates a new manifest.
func NewManifest(
	ctx context.Context,
	manifestType string,
	encrypted bool,
	storer storage.Storer,
) (Interface, error) {
	switch manifestType {
	case ManifestSimpleContentType:
		return NewSimpleManifest(ctx, encrypted, storer)
	default:
		return nil, ErrInvalidManifestType
	}
}

// NewManifestReference loads existing manifest.
func NewManifestReference(
	ctx context.Context,
	manifestType string,
	reference swarm.Address,
	encrypted bool,
	storer storage.Storer,
) (Interface, error) {
	switch manifestType {
	case ManifestSimpleContentType:
		return NewSimpleManifestReference(ctx, reference, encrypted, storer)
	default:
		return nil, ErrInvalidManifestType
	}
}

type manifestEntry struct {
	reference swarm.Address
}

// NewEntry creates a new manifest entry.
func NewEntry(reference swarm.Address) Entry {
	return &manifestEntry{
		reference: reference,
	}
}

func (e *manifestEntry) Reference() swarm.Address {
	return e.reference
}
