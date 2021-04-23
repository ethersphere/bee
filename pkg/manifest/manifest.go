// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package manifest contains the abstractions needed for
// collection representation in Swarm.
package manifest

import (
	"context"
	"errors"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/swarm"
)

const DefaultManifestType = ManifestMantarayContentType

const (
	RootPath                      = "/"
	WebsiteIndexDocumentSuffixKey = "website-index-document"
	WebsiteErrorDocumentPathKey   = "website-error-document"
	EntryMetadataContentTypeKey   = "Content-Type"
	EntryMetadataFilenameKey      = "Filename"
)

var (
	// ErrNotFound is returned when an Entry is not found in the manifest.
	ErrNotFound = errors.New("manifest: not found")

	// ErrInvalidManifestType is returned when an unknown manifest type
	// is provided to the function.
	ErrInvalidManifestType = errors.New("manifest: invalid type")

	// ErrMissingReference is returned when the reference for the manifest file
	// is missing.
	ErrMissingReference = errors.New("manifest: missing reference")
)

// StoreSizeFunc is a callback on every content size that will be stored by
// the Store function.
type StoreSizeFunc func(int64) error

// Interface for operations with manifest.
type Interface interface {
	// Type returns manifest implementation type information
	Type() string
	// Add a manifest entry to the specified path.
	Add(context.Context, string, Entry) error
	// Remove a manifest entry on the specified path.
	Remove(context.Context, string) error
	// Lookup returns a manifest entry if one is found in the specified path.
	Lookup(context.Context, string) (Entry, error)
	// HasPrefix tests whether the specified prefix path exists.
	HasPrefix(context.Context, string) (bool, error)
	// Store stores the manifest, returning the resulting address.
	Store(context.Context, ...StoreSizeFunc) (swarm.Address, error)
	// IterateAddresses is used to iterate over chunks addresses for
	// the manifest.
	IterateAddresses(context.Context, swarm.AddressIterFunc) error
}

// Entry represents a single manifest entry.
type Entry interface {
	// Reference returns the address of the file.
	Reference() swarm.Address
	// Metadata returns the metadata of the file.
	Metadata() map[string]string
}

// NewDefaultManifest creates a new manifest with default type.
func NewDefaultManifest(
	ls file.LoadSaver,
	encrypted bool,
) (Interface, error) {
	return NewManifest(DefaultManifestType, ls, encrypted)
}

// NewDefaultManifestReference creates a new manifest with default type.
func NewDefaultManifestReference(
	reference swarm.Address,
	ls file.LoadSaver,
) (Interface, error) {
	return NewManifestReference(DefaultManifestType, reference, ls)
}

// NewManifest creates a new manifest.
func NewManifest(
	manifestType string,
	ls file.LoadSaver,
	encrypted bool,
) (Interface, error) {
	switch manifestType {
	case ManifestSimpleContentType:
		return NewSimpleManifest(ls)
	case ManifestMantarayContentType:
		return NewMantarayManifest(ls, encrypted)
	default:
		return nil, ErrInvalidManifestType
	}
}

// NewManifestReference loads existing manifest.
func NewManifestReference(
	manifestType string,
	reference swarm.Address,
	ls file.LoadSaver,
) (Interface, error) {
	switch manifestType {
	case ManifestSimpleContentType:
		return NewSimpleManifestReference(reference, ls)
	case ManifestMantarayContentType:
		return NewMantarayManifestReference(reference, ls)
	default:
		return nil, ErrInvalidManifestType
	}
}

type manifestEntry struct {
	reference swarm.Address
	metadata  map[string]string
}

// NewEntry creates a new manifest entry.
func NewEntry(reference swarm.Address, metadata map[string]string) Entry {
	return &manifestEntry{
		reference: reference,
		metadata:  metadata,
	}
}

func (e *manifestEntry) Reference() swarm.Address {
	return e.reference
}

func (e *manifestEntry) Metadata() map[string]string {
	return e.metadata
}
