// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dynamicaccess

import (
	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/kvs"
	kvsmanifest "github.com/ethersphere/bee/pkg/kvs/manifest"
	kvsmemory "github.com/ethersphere/bee/pkg/kvs/memory"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Act represents an interface for accessing and manipulating data.
type Act interface {
	// Add adds a key-value pair to the data store.
	Add(rootHash swarm.Address, key []byte, val []byte) (swarm.Address, error)

	// Lookup retrieves the value associated with the given key from the data store.
	Lookup(rootHash swarm.Address, key []byte) ([]byte, error)

	// Load loads the data store from the given address.
	//Load(addr swarm.Address) error

	// Store stores the current state of the data store and returns the address of the ACT.
	//Store() (swarm.Address, error)
}

// inKvsAct is an implementation of the Act interface that uses kvs storage.
type inKvsAct struct {
	storage kvs.KeyValueStore
}

// Add adds a key-value pair to the in-memory data store.
func (act *inKvsAct) Add(rootHash swarm.Address, key []byte, val []byte) (swarm.Address, error) {
	return act.storage.Put(rootHash, key, val)
}

// Lookup retrieves the value associated with the given key from the in-memory data store.
func (act *inKvsAct) Lookup(rootHash swarm.Address, key []byte) ([]byte, error) {
	return act.storage.Get(rootHash, key)
}

// NewInMemoryAct creates a new instance of the Act interface with in-memory storage.
func NewInMemoryAct() Act {
	s, err := kvs.NewKeyValueStore(nil, kvsmemory.KvsTypeMemory)
	if err != nil {
		return nil
	}
	return &inKvsAct{
		storage: s,
	}
}

// NewInManifestAct creates a new instance of the Act interface with manifest storage.
func NewInManifestAct(storer api.Storer) Act {
	s, err := kvs.NewKeyValueStore(storer, kvsmanifest.KvsTypeManifest)
	if err != nil {
		return nil
	}
	return &inKvsAct{
		storage: s,
	}
}
