// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/crypto"
)

// fuzzPassword is a fixed password shared between seed construction and the
// fuzz target so that the valid seed round-trips successfully.
const fuzzPassword = "swarm-fuzz-password"

// FuzzDecryptKey drives the unexported decryptKey over attacker-controlled
// on-disk keyfile bytes. decryptKey walks the full untrusted decode chain:
// json.Unmarshal into encryptedKey/keyCripto/kdfParams; decryptData's
// hex.DecodeString of MAC/CipherText/IV; getKDFKey's scrypt.Key with N,R,P,DKLen
// read straight from the JSON; aesCTRXOR's make([]byte, len(cipherText)); and
// edg.Decode of the recovered plaintext. It must never panic on any input.
func FuzzDecryptKey(f *testing.F) {
	// (1) Valid seed built the production way so the corpus starts valid.
	k, err := crypto.EDGSecp256_K1.Generate()
	if err != nil {
		f.Fatal(err)
	}
	blob, err := encryptKey(k, fuzzPassword, crypto.EDGSecp256_K1)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(blob)

	// (2) Benign edge seeds.
	f.Add([]byte(``))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"version":3,"crypto":{}}`))
	f.Add([]byte(`not json`))
	f.Add([]byte(`{"version":1,"crypto":{"cipher":"aes-128-ctr"}}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		key, err := decryptKey(data, fuzzPassword, crypto.EDGSecp256_K1)
		// Conditional round-trip invariant: a nil error must yield a usable key.
		if err == nil && key == nil {
			t.Fatalf("decryptKey returned nil error but nil key")
		}
	})
}
