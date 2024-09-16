// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"crypto/ecdsa"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/keystore"
)

type SessionMock struct {
	KeyFunc func(publicKey *ecdsa.PublicKey, nonces [][]byte) ([][]byte, error)
	key     *ecdsa.PrivateKey
}

func (s *SessionMock) Key(publicKey *ecdsa.PublicKey, nonces [][]byte) ([][]byte, error) {
	if s.KeyFunc == nil {
		return nil, nil
	}
	return s.KeyFunc(publicKey, nonces)
}

func NewSessionMock(key *ecdsa.PrivateKey) *SessionMock {
	return &SessionMock{key: key}
}

func NewFromKeystore(
	ks keystore.Service,
	tag,
	password string,
	keyFunc func(publicKey *ecdsa.PublicKey, nonces [][]byte) ([][]byte, error),
) *SessionMock {
	key, created, err := ks.Key(tag, password, crypto.EDGSecp256_K1)
	if !created || err != nil {
		return nil
	}
	return &SessionMock{
		key:     key,
		KeyFunc: keyFunc,
	}
}
