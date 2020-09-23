// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto

import (
	"crypto/ecdsa"
	"errors"
)

// DH is an interface allowing to generate shared keys for public key
// using a salt from a known private key
// TODO: implement clef support beside in-memory
type DH interface {
	SharedKey(public *ecdsa.PublicKey, salt []byte) ([]byte, error)
}

type defaultDH struct {
	key *ecdsa.PrivateKey
}

// NewDH returns an ECDH shared secret key generation seeded with in-memory private key
func NewDH(key *ecdsa.PrivateKey) DH {
	return &defaultDH{key}
}

// SharedKey creates ECDH shared secret using the in-memory key as private key and the given public key
// and hashes it with the salt to return the shared key
// safety warning: this method is not meant to be exposed as it does not validate private and public keys
// are  on the same curve
func (dh *defaultDH) SharedKey(pub *ecdsa.PublicKey, salt []byte) ([]byte, error) {
	x, _ := pub.Curve.ScalarMult(pub.X, pub.Y, dh.key.D.Bytes())
	if x == nil {
		return nil, errors.New("shared secret is point at infinity")
	}
	return LegacyKeccak256(append(x.Bytes(), salt...))
}
