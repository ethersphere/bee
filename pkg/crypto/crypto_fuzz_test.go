// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/crypto"
)

// FuzzDecodeSecp256k1PrivateKey fuzzes crypto.DecodeSecp256k1PrivateKey, which
// decodes raw private-key bytes. The target exercises the Bee-side length guard
// (len != 32 -> error) before btcec.PrivKeyFromBytes(data).ToECDSA(). It asserts
// no input length panics, and that a nil error implies a non-nil key decoded
// from exactly 32 bytes.
func FuzzDecodeSecp256k1PrivateKey(f *testing.F) {
	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		f.Fatal(err)
	}
	b, err := crypto.EncodeSecp256k1PrivateKey(key)
	if err != nil {
		f.Fatal(err)
	}

	f.Add(b)
	f.Add(make([]byte, 32))
	f.Add([]byte{})
	f.Add(make([]byte, 31))
	f.Add(make([]byte, 33))

	f.Fuzz(func(t *testing.T, data []byte) {
		k, err := crypto.DecodeSecp256k1PrivateKey(data)
		if err != nil {
			return
		}
		if k == nil {
			t.Fatal("nil error but nil private key")
		}
		if len(data) != 32 {
			t.Fatalf("nil error but data length %d != 32", len(data))
		}
	})
}
