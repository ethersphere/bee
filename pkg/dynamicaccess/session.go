// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dynamicaccess

import (
	"crypto/ecdsa"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/keystore"
)

// Session represents an interface for a Diffie-Helmann key derivation
type Session interface {
	// Key returns a derived key for each nonce
	Key(publicKey *ecdsa.PublicKey, nonces [][]byte) ([][]byte, error)
}

var _ Session = (*SessionStruct)(nil)

type SessionStruct struct {
	key *ecdsa.PrivateKey
}

var (
	ErrInvalidPublicKey  = errors.New("invalid public key")
	ErrSecretKeyInfinity = errors.New("shared secret is point at infinity")
)

func (s *SessionStruct) Key(publicKey *ecdsa.PublicKey, nonces [][]byte) ([][]byte, error) {
	if publicKey == nil {
		return nil, ErrInvalidPublicKey
	}
	x, y := publicKey.Curve.ScalarMult(publicKey.X, publicKey.Y, s.key.D.Bytes())
	if x == nil || y == nil {
		return nil, ErrSecretKeyInfinity
	}

	if len(nonces) == 0 {
		return [][]byte{(*x).Bytes()}, nil
	}

	keys := make([][]byte, 0, len(nonces))
	for _, nonce := range nonces {
		key, err := crypto.LegacyKeccak256(append(x.Bytes(), nonce...))
		if err != nil {
			return nil, fmt.Errorf("failed to get Keccak256 hash: %w", err)
		}
		keys = append(keys, key)
	}

	return keys, nil
}

func NewDefaultSession(key *ecdsa.PrivateKey) *SessionStruct {
	return &SessionStruct{
		key: key,
	}
}

// Currently implemented only in mock/session.go
func NewFromKeystore(keystore.Service, string, string) Session {
	return nil
}
