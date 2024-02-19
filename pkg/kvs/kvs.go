// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kvs

import (
	"context"
	"encoding/hex"
	"errors"

	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/manifest"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type KeyValueStore interface {
	Get(ctx context.Context, key []byte) ([]byte, error)
	Put(ctx context.Context, key, value []byte) error
	Save(ctx context.Context) (swarm.Address, error)
}

type keyValueStore struct {
	manifest manifest.Interface
	putCnt   int
}

var _ KeyValueStore = (*keyValueStore)(nil)

func (s *keyValueStore) Get(ctx context.Context, key []byte) ([]byte, error) {
	entry, err := s.manifest.Lookup(ctx, hex.EncodeToString(key))
	if err != nil {
		return nil, err
	}
	ref := entry.Reference()
	return ref.Bytes(), nil
}

func (s *keyValueStore) Put(ctx context.Context, key []byte, value []byte) error {
	err := s.manifest.Add(ctx, hex.EncodeToString(key), manifest.NewEntry(swarm.NewAddress(value), map[string]string{}))
	if err != nil {
		return err
	}
	s.putCnt++
	return nil
}

func (s *keyValueStore) Save(ctx context.Context) (swarm.Address, error) {
	if s.putCnt == 0 {
		return swarm.ZeroAddress, errors.New("nothing to save")
	}
	ref, err := s.manifest.Store(ctx)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	s.putCnt = 0
	return ref, nil
}

func New(ls file.LoadSaver) (KeyValueStore, error) {
	m, err := manifest.NewSimpleManifest(ls)
	if err != nil {
		return nil, err
	}

	return &keyValueStore{
		manifest: m,
	}, nil
}

func NewReference(ls file.LoadSaver, ref swarm.Address) (KeyValueStore, error) {
	m, err := manifest.NewSimpleManifestReference(ref, ls)
	if err != nil {
		return nil, err
	}

	return &keyValueStore{
		manifest: m,
	}, nil
}
