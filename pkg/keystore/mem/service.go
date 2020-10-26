// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mem

import (
	"crypto/ecdsa"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/keystore"
)

var _ keystore.Service = (*Service)(nil)

// Service is the memory-based keystore.Service implementation.
//
// Keys are stored in an in-memory map, where the key is the name of the private
// key, and the value is the structure where the actual private key and
// the password are stored.
type Service struct {
	m  map[string]key
	mu sync.Mutex
}

// New creates new memory-based keystore.Service implementation.
func New() *Service {
	return &Service{
		m: make(map[string]key),
	}
}

func (s *Service) Exists(name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.m[name]
	return ok, nil

}

func (s *Service) Key(name, password string) (pk *ecdsa.PrivateKey, created bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	k, ok := s.m[name]
	if !ok {
		pk, err := crypto.GenerateSecp256k1Key()
		if err != nil {
			return nil, false, fmt.Errorf("generate secp256k1 key: %w", err)
		}
		s.m[name] = key{
			pk:       pk,
			password: password,
		}
		return pk, true, nil
	}

	if k.password != password {
		return nil, false, keystore.ErrInvalidPassword
	}

	return k.pk, created, nil
}

type key struct {
	pk       *ecdsa.PrivateKey
	password string
}
