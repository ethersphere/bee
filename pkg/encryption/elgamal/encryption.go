// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package elgamal

import (
	"crypto/ecdsa"
	"hash"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/encryption"
)

func New(key *ecdsa.PrivateKey, pub *ecdsa.PublicKey, salt []byte, padding int, hashfunc func() hash.Hash) (encryption.Interface, error) {
	dh := crypto.NewDH(key)
	sk, err := dh.SharedKey(pub, salt)
	if err != nil {
		return nil, err
	}
	return encryption.New(sk, padding, 0, hashfunc), nil
}

func NewEncryptor(pub *ecdsa.PublicKey, salt []byte, padding int, hashfunc func() hash.Hash) (encryption.Encryptor, *ecdsa.PublicKey, error) {
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		return nil, nil, err
	}
	enc, err := New(privKey, pub, salt, padding, hashfunc)
	if err != nil {
		return nil, nil, err
	}
	return enc, &privKey.PublicKey, nil
}

func NewDecryptor(key *ecdsa.PrivateKey, pub *ecdsa.PublicKey, salt []byte, hashfunc func() hash.Hash) (encryption.Decryptor, error) {
	return New(key, pub, salt, 0, hashfunc)
}
