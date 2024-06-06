// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accesscontrol

import (
	"crypto/ecdsa"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/crypto"
)

var (
	// ErrInvalidPublicKey is an error that is returned when a public key is nil.
	ErrInvalidPublicKey = errors.New("invalid public key")
	// ErrSecretKeyInfinity is an error that is returned when the shared secret is a point at infinity.
	ErrSecretKeyInfinity = errors.New("shared secret is point at infinity")
)

// Session represents an interface for a Diffie-Hellmann key derivation
type Session interface {
	// Key returns a derived key for each nonce.
	Key(publicKey *ecdsa.PublicKey, nonces [][]byte) ([][]byte, error)
}

var _ Session = (*SessionStruct)(nil)

// SessionStruct represents a session with an access control key.
type SessionStruct struct {
	key *ecdsa.PrivateKey
}

// Key returns a derived key for each nonce.
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

// NewDefaultSession creates a new session from a private key.
func NewDefaultSession(key *ecdsa.PrivateKey) *SessionStruct {
	return &SessionStruct{
		key: key,
	}
}
