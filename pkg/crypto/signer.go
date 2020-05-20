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

type Recoverer interface {
	Recover(signature, data []byte) (*ecdsa.PublicKey, error)
}

type SignRecoverer interface {
	Signer
	Recoverer
}

type defaultRecoverer struct{}

func (d defaultRecoverer) Recover(signature, data []byte) (*ecdsa.PublicKey, error) {
	p, _, err := btcec.RecoverCompact(btcec.S256(), signature, data)
	return (*ecdsa.PublicKey)(p), err
}

type defaultSigner struct {
	key       *ecdsa.PrivateKey
	recoverer Recoverer
}

func NewDefaultSigner(key *ecdsa.PrivateKey) SignRecoverer {
	return &defaultSigner{
		key:       key,
		recoverer: defaultRecoverer{},
	}
}

func (d *defaultSigner) PublicKey() (*ecdsa.PublicKey, error) {
	return &d.key.PublicKey, nil
}

func (d *defaultSigner) Sign(data []byte) (signature []byte, err error) {
	return btcec.SignCompact(btcec.S256(), (*btcec.PrivateKey)(d.key), data, true)
}

func (d *defaultSigner) Recover(signature, data []byte) (*ecdsa.PublicKey, error) {
	return d.recoverer.Recover(signature, data)
}
