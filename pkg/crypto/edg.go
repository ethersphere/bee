// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto

import "crypto/ecdsa"

// Secp256k1EDG aggregates private key cryptography functions that employ secp256k1
type Secp256k1EDG struct{}

func (s *Secp256k1EDG) Generate() (*ecdsa.PrivateKey, error) {
	return GenerateSecp256k1Key()
}
func (s *Secp256k1EDG) Encode(k *ecdsa.PrivateKey) ([]byte, error) {
	return EncodeSecp256k1PrivateKey(k)
}
func (s *Secp256k1EDG) Decode(data []byte) (*ecdsa.PrivateKey, error) {
	return DecodeSecp256k1PrivateKey(data)
}

// Secp256r1EDG aggregates private key cryptography functions that employ secp256r1
type Secp256r1EDG struct{}

func (s *Secp256r1EDG) Generate() (*ecdsa.PrivateKey, error) {
	return GenerateSecp256r1Key()
}
func (s *Secp256r1EDG) Encode(k *ecdsa.PrivateKey) ([]byte, error) {
	return EncodeSecp256r1PrivateKey(k)
}
func (s *Secp256r1EDG) Decode(data []byte) (*ecdsa.PrivateKey, error) {
	return DecodeSecp256r1PrivateKey(data)
}
