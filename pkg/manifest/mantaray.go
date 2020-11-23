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
	"github.com/ethersphere/manifest/mantaray"
)

const (
	// ManifestMantarayContentType represents content type used for noting that
	// specific file should be processed as mantaray manifest.
	ManifestMantarayContentType = "application/bzz-manifest-mantaray+octet-stream"
)

type mantarayManifest struct {
	trie *mantaray.Node

	encrypted bool
	storer    storage.Storer

	loader mantaray.LoadSaver
}

// NewMantarayManifest creates a new mantaray-based manifest.
func NewMantarayManifest(
	encrypted bool,
	storer storage.Storer,
) (Interface, error) {
	return &mantarayManifest{
		trie:      mantaray.New(),
		encrypted: encrypted,
		storer:    storer,
	}, nil
}

// NewMantarayManifestWithObfuscationKeyFn creates a new mantaray-based manifest
// with configured obfuscation key
//
// NOTE: This should only be used in tests.
func NewMantarayManifestWithObfuscationKeyFn(
	encrypted bool,
	storer storage.Storer,
	obfuscationKeyFn func([]byte) (int, error),
) (Interface, error) {
	mm := &mantarayManifest{
		trie:      mantaray.New(),
		encrypted: encrypted,
		storer:    storer,
	}
	mantaray.SetObfuscationKeyFn(obfuscationKeyFn)
	return mm, nil
}

// NewMantarayManifestReference loads existing mantaray-based manifest.
func NewMantarayManifestReference(
	ctx context.Context,
	reference swarm.Address,
	encrypted bool,
	storer storage.Storer,
) (Interface, error) {
	return &mantarayManifest{
		trie:      mantaray.NewNodeRef(reference.Bytes()),
		encrypted: encrypted,
		storer:    storer,
		loader:    newMantarayLoader(ctx, encrypted, storer),
	}, nil
}

func (m *mantarayManifest) Type() string {
	return ManifestMantarayContentType
}

func (m *mantarayManifest) Add(path string, entry Entry) error {
	p := []byte(path)
	e := entry.Reference().Bytes()

	return m.trie.Add(p, e, entry.Metadata(), m.loader)
}

func (m *mantarayManifest) Remove(path string) error {
	p := []byte(path)

	err := m.trie.Remove(p, m.loader)
	if err != nil {
		if errors.Is(err, mantaray.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	return nil
}

func (m *mantarayManifest) Lookup(path string) (Entry, error) {
	p := []byte(path)

	node, err := m.trie.LookupNode(p, m.loader)
	if err != nil {
		if errors.Is(err, mantaray.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	if !node.IsValueType() {
		return nil, ErrNotFound
	}

	address := swarm.NewAddress(node.Entry())

	entry := NewEntry(address, node.Metadata())

	return entry, nil
}

func (m *mantarayManifest) HasPrefix(prefix string) (bool, error) {
	p := []byte(prefix)

	return m.trie.HasPrefix(p, m.loader)
}

func (m *mantarayManifest) Store(ctx context.Context, mode storage.ModePut) (swarm.Address, error) {

	saver := newMantaraySaver(ctx, m.encrypted, m.storer, mode)
	m.loader = saver

	err := m.trie.Save(saver)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("manifest save error: %w", err)
	}

	address := swarm.NewAddress(m.trie.Reference())

	return address, nil
}

func (m *mantarayManifest) IterateAddresses(ctx context.Context, fn swarm.AddressIterFunc) error {
	reference := swarm.NewAddress(m.trie.Reference())

	if swarm.ZeroAddress.Equal(reference) {
		return ErrMissingReference
	}

	walker := func(path []byte, node *mantaray.Node, err error) error {
		if err != nil {
			return err
		}

		if node != nil {
			var stop bool

			if node.Reference() != nil {
				ref := swarm.NewAddress(node.Reference())

				stop = fn(ref)
				if stop {
					return errStopIterator
				}
			}

			if node.IsValueType() && node.Entry() != nil {
				entry := swarm.NewAddress(node.Entry())
				stop = fn(entry)
				if stop {
					return errStopIterator
				}
			}
		}

		return nil
	}

	err := m.trie.WalkNode([]byte{}, m.loader, walker)
	if err != nil {
		if !errors.Is(err, errStopIterator) {
			return fmt.Errorf("manifest iterate addresses: %w", err)
		}
		// ignore error if interation stopped by caller
	}

	return nil
}

// mantarayLoadSaver implements required interface 'mantaray.LoadSaver'
type mantarayLoadSaver struct {
	ctx       context.Context
	encrypted bool
	storer    storage.Storer
	modePut   storage.ModePut
}

func newMantarayLoader(
	ctx context.Context,
	encrypted bool,
	storer storage.Storer,
) *mantarayLoadSaver {
	return &mantarayLoadSaver{
		ctx:       ctx,
		encrypted: encrypted,
		storer:    storer,
	}
}

func newMantaraySaver(
	ctx context.Context,
	encrypted bool,
	storer storage.Storer,
	modePut storage.ModePut,
) *mantarayLoadSaver {
	return &mantarayLoadSaver{
		ctx:       ctx,
		encrypted: encrypted,
		storer:    storer,
		modePut:   modePut,
	}
}

func (ls *mantarayLoadSaver) Load(ref []byte) ([]byte, error) {
	ctx := ls.ctx

	j, _, err := joiner.New(ctx, ls.storer, swarm.NewAddress(ref))
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(nil)
	_, err = file.JoinReadAll(ctx, j, buf)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (ls *mantarayLoadSaver) Save(data []byte) ([]byte, error) {
	ctx := ls.ctx

	pipe := builder.NewPipelineBuilder(ctx, ls.storer, ls.modePut, ls.encrypted)
	address, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(data), int64(len(data)))

	if err != nil {
		return swarm.ZeroAddress.Bytes(), err
	}

	return address.Bytes(), nil
}
