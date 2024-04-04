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
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type KeyValueStore interface {
	Get(key []byte) ([]byte, error)
	Put(key, value []byte) error
	Save() (swarm.Address, error)
}

type keyValueStore struct {
	manifest manifest.Interface
	putter   storer.PutterSession
	putCnt   int
}

var _ KeyValueStore = (*keyValueStore)(nil)

// TODO: pass context as dep.
func (s *keyValueStore) Get(key []byte) ([]byte, error) {
	entry, err := s.manifest.Lookup(context.Background(), hex.EncodeToString(key))
	if err != nil {
		return nil, err
	}
	ref := entry.Reference()
	return ref.Bytes(), nil
}

func (s *keyValueStore) Put(key []byte, value []byte) error {
	err := s.manifest.Add(context.Background(), hex.EncodeToString(key), manifest.NewEntry(swarm.NewAddress(value), map[string]string{}))
	if err != nil {
		return err
	}
	s.putCnt++
	return nil
}

func (s *keyValueStore) Save() (swarm.Address, error) {
	if s.putCnt == 0 {
		return swarm.ZeroAddress, errors.New("nothing to save")
	}
	ref, err := s.manifest.Store(context.Background())
	if err != nil {
		return swarm.ZeroAddress, err
	}
	err = s.putter.Done(ref)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	s.putCnt = 0
	return ref, nil
}

func New(ls file.LoadSaver, putter storer.PutterSession, rootHash swarm.Address) KeyValueStore {
	var (
		manif manifest.Interface
		err   error
	)
	if swarm.ZeroAddress.Equal(rootHash) || swarm.EmptyAddress.Equal(rootHash) {
		manif, err = manifest.NewSimpleManifest(ls)
	} else {
		manif, err = manifest.NewSimpleManifestReference(rootHash, ls)
	}
	if err != nil {
		return nil
	}

	return &keyValueStore{
		manifest: manif,
		putter:   putter,
	}
}
