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

// New constructs an encryption interface (the modified blockcipher) with a base key derived from
// a shared secret (using a private key and the counterparty's public key) hashed with  a salt
func New(key *ecdsa.PrivateKey, pub *ecdsa.PublicKey, salt []byte, padding int, hashfunc func() hash.Hash) (encryption.Interface, error) {
	dh := crypto.NewDH(key)
	sk, err := dh.SharedKey(pub, salt)
	if err != nil {
		return nil, err
	}
	return encryption.New(sk, padding, 0, hashfunc), nil
}

// NewEncryptor constructs an El-Gamal encryptor
// this involves generating an ephemeral key pair the public part of which is returned
// as it is needed for the counterparty to decrypt
func NewEncryptor(pub *ecdsa.PublicKey, salt []byte, padding int, hashfunc func() hash.Hash) (encryption.Encrypter, *ecdsa.PublicKey, error) {
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

// NewDecrypter constructs an el-Gamal decrypter the receiving party uses
// the public key must be the ephemeral return value of the Encrypter constructor
func NewDecrypter(key *ecdsa.PrivateKey, pub *ecdsa.PublicKey, salt []byte, hashfunc func() hash.Hash) (encryption.Decrypter, error) {
	return New(key, pub, salt, 0, hashfunc)
}
