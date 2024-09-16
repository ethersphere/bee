// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"crypto/ecdsa"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type GranteeListMock interface {
	Add(publicKeys []*ecdsa.PublicKey) error
	Remove(removeList []*ecdsa.PublicKey) error
	Get() []*ecdsa.PublicKey
	Save() (swarm.Address, error)
}

type GranteeListStructMock struct {
	grantees []*ecdsa.PublicKey
}

func (g *GranteeListStructMock) Get() []*ecdsa.PublicKey {
	grantees := g.grantees
	keys := make([]*ecdsa.PublicKey, len(grantees))
	copy(keys, grantees)
	return keys
}

func (g *GranteeListStructMock) Add(addList []*ecdsa.PublicKey) error {
	g.grantees = append(g.grantees, addList...)
	return nil
}

func (g *GranteeListStructMock) Remove(removeList []*ecdsa.PublicKey) error {
	for _, remove := range removeList {
		for i, grantee := range g.grantees {
			if *grantee == *remove {
				g.grantees[i] = g.grantees[len(g.grantees)-1]
				g.grantees = g.grantees[:len(g.grantees)-1]
			}
		}
	}

	return nil
}

func (g *GranteeListStructMock) Save() (swarm.Address, error) {
	return swarm.EmptyAddress, nil
}

func NewGranteeList() *GranteeListStructMock {
	return &GranteeListStructMock{grantees: []*ecdsa.PublicKey{}}
}
