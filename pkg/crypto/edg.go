// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto

import (
	"crypto/ecdsa"

	"github.com/ethersphere/bee/pkg/keystore"
)

// EDGSecp256_k1 aggregates private key cryptography functions that employ secp256k1
type EDGSecp256_k1 struct{}

var EDGSecp256_K1 keystore.EDG = new(EDGSecp256_k1)

func (s *EDGSecp256_k1) Generate() (*ecdsa.PrivateKey, error) {
	return GenerateSecp256k1Key()
}
func (s *EDGSecp256_k1) Encode(k *ecdsa.PrivateKey) ([]byte, error) {
	return EncodeSecp256k1PrivateKey(k)
}
func (s *EDGSecp256_k1) Decode(data []byte) (*ecdsa.PrivateKey, error) {
	return DecodeSecp256k1PrivateKey(data)
}

// EDGSecp256_r1 aggregates private key cryptography functions that employ secp256r1
type EDGSecp256_r1 struct{}

var EDGSecp256_R1 keystore.EDG = new(EDGSecp256_r1)

func (s *EDGSecp256_r1) Generate() (*ecdsa.PrivateKey, error) {
	return GenerateSecp256r1Key()
}
func (s *EDGSecp256_r1) Encode(k *ecdsa.PrivateKey) ([]byte, error) {
	return EncodeSecp256r1PrivateKey(k)
}
func (s *EDGSecp256_r1) Decode(data []byte) (*ecdsa.PrivateKey, error) {
	return DecodeSecp256r1PrivateKey(data)
}
