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
	"github.com/ethersphere/bee/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/pkg/file/seekjoiner"
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

	err := m.trie.Save(saver)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("manifest save error: %w", err)
	}

	address := swarm.NewAddress(m.trie.Reference())

	return address, nil
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

	j := seekjoiner.NewSimpleJoiner(ls.storer)

	buf := bytes.NewBuffer(nil)
	_, err := file.JoinReadAll(ctx, j, swarm.NewAddress(ref), buf)
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
