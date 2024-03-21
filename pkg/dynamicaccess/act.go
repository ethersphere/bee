// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dynamicaccess

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/swarm"
)

var lock = &sync.Mutex{}

type single struct {
	memoryMock map[string]manifest.Entry
}

var singleInMemorySwarm *single

func getInMemorySwarm() *single {
	if singleInMemorySwarm == nil {
		lock.Lock()
		defer lock.Unlock()
		if singleInMemorySwarm == nil {
			singleInMemorySwarm = &single{
				memoryMock: make(map[string]manifest.Entry)}
		}
	}
	return singleInMemorySwarm
}

func getMemory() map[string]manifest.Entry {
	ch := make(chan *single)
	go func() {
		ch <- getInMemorySwarm()
	}()
	mem := <-ch
	return mem.memoryMock
}

// Act represents an interface for accessing and manipulating data.
type Act interface {
	// Add adds a key-value pair to the data store.
	Add(key []byte, val []byte) error

	// Lookup retrieves the value associated with the given key from the data store.
	Lookup(key []byte) ([]byte, error)

	// Load loads the data store from the given address.
	Load(addr swarm.Address) error

	// Store stores the current state of the data store and returns the address of the ACT.
	Store() (swarm.Address, error)
}

var _ Act = (*inMemoryAct)(nil)

// inMemoryAct is a simple implementation of the Act interface, with in memory storage.
type inMemoryAct struct {
	container map[string]string
}

func (act *inMemoryAct) Add(key []byte, val []byte) error {
	act.container[hex.EncodeToString(key)] = hex.EncodeToString(val)
	return nil
}

func (act *inMemoryAct) Lookup(key []byte) ([]byte, error) {
	if key, ok := act.container[hex.EncodeToString(key)]; ok {
		bytes, err := hex.DecodeString(key)
		if err != nil {
			return nil, err
		}
		return bytes, nil
	}
	return nil, fmt.Errorf("key not found")
}

func (act *inMemoryAct) Load(addr swarm.Address) error {
	memory := getMemory()
	me := memory[addr.String()]
	if me == nil {
		return fmt.Errorf("ACT not found at address: %s", addr.String())
	}
	act.container = me.Metadata()
	return nil
}

func (act *inMemoryAct) Store() (swarm.Address, error) {
	// Generate a random swarm.Address
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return swarm.EmptyAddress, fmt.Errorf("failed to generate random address: %w", err)
	}
	swarm_ref := swarm.NewAddress(b)
	mem := getMemory()
	mem[swarm_ref.String()] = manifest.NewEntry(swarm_ref, act.container)

	return swarm_ref, nil
}

func NewInMemoryAct() Act {
	return &inMemoryAct{
		container: make(map[string]string),
	}
}
