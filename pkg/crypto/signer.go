// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto

import (
	"crypto/ecdsa"

	"github.com/btcsuite/btcd/btcec"
)

type Signer interface {
	Sign(data []byte) ([]byte, error)
	PublicKey() (*ecdsa.PublicKey, error)
}

// Recover verifies signature with the data base provided.
// It is using `btcec.RecoverCompact` function
func Recover(signature, data []byte) (*ecdsa.PublicKey, error) {
	p, _, err := btcec.RecoverCompact(btcec.S256(), signature, data)
	return (*ecdsa.PublicKey)(p), err
}

type defaultSigner struct {
	key *ecdsa.PrivateKey
}

func NewDefaultSigner(key *ecdsa.PrivateKey) Signer {
	return &defaultSigner{
		key: key,
	}
}

func (d *defaultSigner) PublicKey() (*ecdsa.PublicKey, error) {
	return &d.key.PublicKey, nil
}

func (d *defaultSigner) Sign(data []byte) (signature []byte, err error) {
	return btcec.SignCompact(btcec.S256(), (*btcec.PrivateKey)(d.key), data, true)
}
