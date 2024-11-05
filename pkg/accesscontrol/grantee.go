// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accesscontrol

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	publicKeyLen = 65
)

var (
	// ErrNothingToRemove indicates that the remove list is empty.
	ErrNothingToRemove = errors.New("nothing to remove")
	// ErrNoGranteeFound indicates that the grantee list is empty.
	ErrNoGranteeFound = errors.New("no grantee found")
	// ErrNothingToAdd indicates that the add list is empty.
	ErrNothingToAdd = errors.New("nothing to add")
)

// GranteeList manages a list of public keys.
type GranteeList interface {
	// Add adds a list of public keys to the grantee list. It filters out duplicates.
	Add(addList []*ecdsa.PublicKey) error
	// Remove removes a list of public keys from the grantee list, if there is any.
	Remove(removeList []*ecdsa.PublicKey) error
	// Get simply returns the list of public keys.
	Get() []*ecdsa.PublicKey
	// Save saves the grantee list to the underlying storage and returns the reference.
	Save(ctx context.Context) (swarm.Address, error)
}

// GranteeListStruct represents a list of grantee public keys.
type GranteeListStruct struct {
	grantees []*ecdsa.PublicKey
	loadSave file.LoadSaver
}

var _ GranteeList = (*GranteeListStruct)(nil)

// Get simply returns the list of public keys.
func (g *GranteeListStruct) Get() []*ecdsa.PublicKey {
	return g.grantees
}

// Add adds a list of public keys to the grantee list. It filters out duplicates.
func (g *GranteeListStruct) Add(addList []*ecdsa.PublicKey) error {
	if len(addList) == 0 {
		return ErrNothingToAdd
	}
	filteredList := make([]*ecdsa.PublicKey, 0, len(addList))
	for _, addkey := range addList {
		add := true
		for _, granteekey := range g.grantees {
			if granteekey.Equal(addkey) {
				add = false
				break
			}
		}
		for _, filteredkey := range filteredList {
			if filteredkey.Equal(addkey) {
				add = false
				break
			}
		}
		if add {
			filteredList = append(filteredList, addkey)
		}
	}
	g.grantees = append(g.grantees, filteredList...)

	return nil
}

// Save saves the grantee list to the underlying storage and returns the reference.
func (g *GranteeListStruct) Save(ctx context.Context) (swarm.Address, error) {
	data, err := serialize(g.grantees)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("grantee serialize error: %w", err)
	}
	refBytes, err := g.loadSave.Save(ctx, data)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("grantee save error: %w", err)
	}

	return swarm.NewAddress(refBytes), nil
}

// Remove removes a list of public keys from the grantee list, if there is any.
func (g *GranteeListStruct) Remove(keysToRemove []*ecdsa.PublicKey) error {
	if len(keysToRemove) == 0 {
		return ErrNothingToRemove
	}

	if len(g.grantees) == 0 {
		return ErrNoGranteeFound
	}
	grantees := g.grantees

	for _, remove := range keysToRemove {
		for i := 0; i < len(grantees); i++ {
			if grantees[i].Equal(remove) {
				grantees[i] = grantees[len(grantees)-1]
				grantees = grantees[:len(grantees)-1]
			}
		}
	}
	g.grantees = grantees

	return nil
}

// NewGranteeList creates a new (and empty) grantee list.
func NewGranteeList(ls file.LoadSaver) *GranteeListStruct {
	return &GranteeListStruct{
		grantees: []*ecdsa.PublicKey{},
		loadSave: ls,
	}
}

// NewGranteeListReference loads an existing grantee list.
func NewGranteeListReference(ctx context.Context, ls file.LoadSaver, reference swarm.Address) (*GranteeListStruct, error) {
	data, err := ls.Load(ctx, reference.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to load grantee list reference, %w", err)
	}
	grantees := deserialize(data)

	return &GranteeListStruct{
		grantees: grantees,
		loadSave: ls,
	}, nil
}

func serialize(publicKeys []*ecdsa.PublicKey) ([]byte, error) {
	b := make([]byte, 0, len(publicKeys)*publicKeyLen)
	for _, key := range publicKeys {
		// TODO: check if this is the correct way to serialize the public key
		// Is this the only curve we support?
		// Should we have switch case for different curves?
		pubBytes := crypto.S256().Marshal(key.X, key.Y)
		b = append(b, pubBytes...)
	}
	return b, nil
}

func deserialize(data []byte) []*ecdsa.PublicKey {
	if len(data) == 0 {
		return []*ecdsa.PublicKey{}
	}

	p := make([]*ecdsa.PublicKey, 0, len(data)/publicKeyLen)
	for i := 0; i < len(data); i += publicKeyLen {
		pubKey := deserializeBytes(data[i : i+publicKeyLen])
		if pubKey == nil {
			return []*ecdsa.PublicKey{}
		}
		p = append(p, pubKey)
	}
	return p
}

func deserializeBytes(data []byte) *ecdsa.PublicKey {
	key, err := btcec.ParsePubKey(data)
	if err != nil {
		return nil
	}
	return key.ToECDSA()
}
