// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package manifest

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/manifest/simple"
)

const (
	// ManifestSimpleContentType represents content type used for noting that
	// specific file should be processed as 'simple' manifest
	ManifestSimpleContentType = "application/bzz-manifest-simple+json"
)

type simpleManifest struct {
	manifest simple.Manifest

	reference swarm.Address
	ls        file.LoadSaver
}

// NewSimpleManifest creates a new simple manifest.
func NewSimpleManifest(ls file.LoadSaver) (Interface, error) {
	return &simpleManifest{
		manifest: simple.NewManifest(),
		ls:       ls,
	}, nil
}

// NewSimpleManifestReference loads existing simple manifest.
func NewSimpleManifestReference(ref swarm.Address, l file.LoadSaver) (Interface, error) {
	m := &simpleManifest{
		manifest:  simple.NewManifest(),
		reference: ref,
		ls:        l,
	}
	err := m.load(context.Background(), ref)
	return m, err
}

func (m *simpleManifest) Type() string {
	return ManifestSimpleContentType
}

func (m *simpleManifest) Add(_ context.Context, path string, entry Entry) error {
	e := entry.Reference().String()

	return m.manifest.Add(path, e, entry.Metadata())
}

func (m *simpleManifest) Remove(_ context.Context, path string) error {
	err := m.manifest.Remove(path)
	if err != nil {
		if errors.Is(err, simple.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	return nil
}

func (m *simpleManifest) Lookup(_ context.Context, path string) (Entry, error) {
	n, err := m.manifest.Lookup(path)
	if err != nil {
		return nil, ErrNotFound
	}

	address, err := swarm.ParseHexAddress(n.Reference())
	if err != nil {
		return nil, fmt.Errorf("parse swarm address: %w", err)
	}

	entry := NewEntry(address, n.Metadata())

	return entry, nil
}

func (m *simpleManifest) HasPrefix(_ context.Context, prefix string) (bool, error) {
	return m.manifest.HasPrefix(prefix), nil
}

func (m *simpleManifest) Store(ctx context.Context, storeSizeFn ...StoreSizeFunc) (swarm.Address, error) {
	data, err := m.manifest.MarshalBinary()
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("manifest marshal error: %w", err)
	}

	if len(storeSizeFn) > 0 {
		dataLen := int64(len(data))
		for i := range storeSizeFn {
			err = storeSizeFn[i](dataLen)
			if err != nil {
				return swarm.ZeroAddress, fmt.Errorf("manifest store size func: %w", err)
			}
		}
	}

	ref, err := m.ls.Save(ctx, data)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("manifest save error: %w", err)
	}
	m.reference = swarm.NewAddress(ref)
	return m.reference, nil
}

func (m *simpleManifest) IterateAddresses(ctx context.Context, fn swarm.AddressIterFunc) error {
	if swarm.ZeroAddress.Equal(m.reference) {
		return ErrMissingReference
	}

	// NOTE: making it behave same for all manifest implementation
	err := fn(m.reference)
	if err != nil {
		return fmt.Errorf("manifest iterate addresses: %w", err)
	}

	walker := func(path string, entry simple.Entry, err error) error {
		if err != nil {
			return err
		}

		ref, err := swarm.ParseHexAddress(entry.Reference())
		if err != nil {
			return err
		}

		return fn(ref)
	}

	err = m.manifest.WalkEntry("", walker)
	if err != nil {
		return fmt.Errorf("manifest iterate addresses: %w", err)
	}

	return nil
}

func (m *simpleManifest) load(ctx context.Context, reference swarm.Address) error {
	buf, err := m.ls.Load(ctx, reference.Bytes())
	if err != nil {
		return fmt.Errorf("manifest load error: %w", err)
	}

	err = m.manifest.UnmarshalBinary(buf)
	if err != nil {
		return fmt.Errorf("manifest unmarshal error: %w", err)
	}

	return nil
}
