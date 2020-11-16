// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package manifest

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/pkg/storage"
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
	encrypted bool
	storer    storage.Storer
}

// NewSimpleManifest creates a new simple manifest.
func NewSimpleManifest(
	encrypted bool,
	storer storage.Storer,
) (Interface, error) {
	return &simpleManifest{
		manifest:  simple.NewManifest(),
		encrypted: encrypted,
		storer:    storer,
	}, nil
}

// NewSimpleManifestReference loads existing simple manifest.
func NewSimpleManifestReference(
	ctx context.Context,
	reference swarm.Address,
	encrypted bool,
	storer storage.Storer,
) (Interface, error) {
	m := &simpleManifest{
		manifest:  simple.NewManifest(),
		reference: reference,
		encrypted: encrypted,
		storer:    storer,
	}
	err := m.load(ctx, reference)
	return m, err
}

func (m *simpleManifest) Type() string {
	return ManifestSimpleContentType
}

func (m *simpleManifest) Add(path string, entry Entry) error {
	e := entry.Reference().String()

	return m.manifest.Add(path, e, entry.Metadata())
}

func (m *simpleManifest) Remove(path string) error {

	err := m.manifest.Remove(path)
	if err != nil {
		if errors.Is(err, simple.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	return nil
}

func (m *simpleManifest) Lookup(path string) (Entry, error) {

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

func (m *simpleManifest) HasPrefix(prefix string) (bool, error) {
	return m.manifest.HasPrefix(prefix), nil
}

func (m *simpleManifest) Store(ctx context.Context, mode storage.ModePut) (swarm.Address, error) {

	data, err := m.manifest.MarshalBinary()
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("manifest marshal error: %w", err)
	}

	pipe := builder.NewPipelineBuilder(ctx, m.storer, mode, m.encrypted)
	address, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("manifest save error: %w", err)
	}

	m.reference = address

	return address, nil
}

func (m *simpleManifest) IterateAddresses(ctx context.Context, fn swarm.AddressIterFunc) error {
	if swarm.ZeroAddress.Equal(m.reference) {
		return ErrMissingReference
	}

	// NOTE: making it behave same for all manifest implementation
	stop := fn(m.reference)
	if stop {
		return nil
	}

	walker := func(path string, entry simple.Entry, err error) error {
		if err != nil {
			return err
		}

		ref, err := swarm.ParseHexAddress(entry.Reference())
		if err != nil {
			return err
		}

		stop := fn(ref)
		if stop {
			return errStopIterator
		}

		return nil
	}

	err := m.manifest.WalkEntry("", walker)
	if err != nil {
		if !errors.Is(err, errStopIterator) {
			return fmt.Errorf("manifest iterate addresses: %w", err)
		}
		// ignore error if interation stopped by caller
	}

	return nil
}

func (m *simpleManifest) load(ctx context.Context, reference swarm.Address) error {
	j, _, err := joiner.New(ctx, m.storer, reference)
	if err != nil {
		return fmt.Errorf("new joiner: %w", err)
	}

	buf := bytes.NewBuffer(nil)
	_, err = file.JoinReadAll(ctx, j, buf)
	if err != nil {
		return fmt.Errorf("manifest load error: %w", err)
	}

	err = m.manifest.UnmarshalBinary(buf.Bytes())
	if err != nil {
		return fmt.Errorf("manifest unmarshal error: %w", err)
	}

	return nil
}
