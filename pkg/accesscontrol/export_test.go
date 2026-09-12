// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accesscontrol

import "crypto/ecdsa"

// PublicKeyLen exposes the serialized secp256k1 public key length for fuzzing.
const PublicKeyLen = publicKeyLen

// Deserialize exposes the unexported grantee-list blob parser for fuzzing.
func Deserialize(data []byte) []*ecdsa.PublicKey {
	return deserialize(data)
}

// Serialize exposes the unexported grantee-list serializer for fuzzing.
func Serialize(publicKeys []*ecdsa.PublicKey) ([]byte, error) {
	return serialize(publicKeys)
}
