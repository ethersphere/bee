// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package manifest

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/manifest/mantaray"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	// ManifestMantarayContentType represents content type used for noting that
	// specific file should be processed as mantaray manifest.
	ManifestMantarayContentType = "application/bzz-manifest-mantaray+octet-stream"
)

type mantarayManifest struct {
	trie *mantaray.Node

	ls file.LoadSaver
}

// NewMantarayManifest creates a new mantaray-based manifest.
func NewMantarayManifest(
	ls file.LoadSaver,
	encrypted bool,
) (Interface, error) {
	mm := &mantarayManifest{
		trie: mantaray.New(),
		ls:   ls,
	}
	// use empty obfuscation key if not encrypting
	if !encrypted {
		// NOTE: it will be copied to all trie nodes
		mm.trie.SetObfuscationKey(mantaray.ZeroObfuscationKey)
	}
	return mm, nil
}

// NewMantarayManifestReference loads existing mantaray-based manifest.
func NewMantarayManifestReference(
	reference swarm.Address,
	ls file.LoadSaver,
) (Interface, error) {
	return &mantarayManifest{
		trie: mantaray.NewNodeRef(reference.Bytes()),
		ls:   ls,
	}, nil
}

func (m *mantarayManifest) Type() string {
	return ManifestMantarayContentType
}

func (m *mantarayManifest) Add(ctx context.Context, path string, entry Entry) error {
	p := []byte(path)
	e := entry.Reference().Bytes()

	return m.trie.Add(ctx, p, e, entry.Metadata(), m.ls)
}

func (m *mantarayManifest) Remove(ctx context.Context, path string) error {
	p := []byte(path)

	err := m.trie.Remove(ctx, p, m.ls)
	if err != nil {
		if errors.Is(err, mantaray.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	return nil
}

func (m *mantarayManifest) Lookup(ctx context.Context, path string) (Entry, error) {
	p := []byte(path)

	node, err := m.trie.LookupNode(ctx, p, m.ls)
	if err != nil {
		if errors.Is(err, mantaray.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if !node.IsValueType() {
		return nil, ErrNotFound
	}

	address := swarm.NewAddress(node.Entry())
	entry := NewEntry(address, node.Metadata())

	return entry, nil
}

func (m *mantarayManifest) HasPrefix(ctx context.Context, prefix string) (bool, error) {
	p := []byte(prefix)

	return m.trie.HasPrefix(ctx, p, m.ls)
}

func (m *mantarayManifest) Store(ctx context.Context, storeSizeFn ...StoreSizeFunc) (swarm.Address, error) {
	var ls mantaray.LoadSaver
	if len(storeSizeFn) > 0 {
		ls = &mantarayLoadSaver{
			ls:          m.ls,
			storeSizeFn: storeSizeFn,
		}
	} else {
		ls = m.ls
	}

	err := m.trie.Save(ctx, ls)
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
			if node.Reference() != nil {
				ref := swarm.NewAddress(node.Reference())

				err = fn(ref)
				if err != nil {
					return err
				}
			}

			if node.IsValueType() && node.Entry() != nil && len(node.Entry()) > 0 {
				entry := swarm.NewAddress(node.Entry())
				err = fn(entry)
				if err != nil {
					return err
				}
			}
		}

		return nil
	}

	err := m.trie.WalkNode(ctx, []byte{}, m.ls, walker)
	if err != nil {
		return fmt.Errorf("manifest iterate addresses: %w", err)
	}

	return nil
}

type mantarayLoadSaver struct {
	ls          file.LoadSaver
	storeSizeFn []StoreSizeFunc
}

func (ls *mantarayLoadSaver) Load(ctx context.Context, ref []byte) ([]byte, error) {
	return ls.ls.Load(ctx, ref)
}

func (ls *mantarayLoadSaver) Save(ctx context.Context, data []byte) ([]byte, error) {
	dataLen := int64(len(data))
	for i := range ls.storeSizeFn {
		err := ls.storeSizeFn[i](dataLen)
		if err != nil {
			return nil, fmt.Errorf("manifest store size func: %w", err)
		}
	}

	return ls.ls.Save(ctx, data)
}
