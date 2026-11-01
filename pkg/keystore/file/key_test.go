// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/crypto"
)

func TestKDFParamsValidate(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		param kdfParams
		valid bool
	}{
		{"production", kdfParams{N: scryptN, R: scryptR, P: scryptP, DKLen: scryptDKLen}, true},
		{"minimal", kdfParams{N: 2, R: 1, P: 1, DKLen: scryptDKLen}, true},
		{"zero", kdfParams{}, false},
		{"missing dklen", kdfParams{N: 2, R: 1, P: 1}, false},
		{"short dklen", kdfParams{N: 2, R: 1, P: 1, DKLen: scryptDKLen - 1}, false},
		{"huge dklen", kdfParams{N: 2, R: 1, P: 1, DKLen: maxScryptDKLen + 1}, false},
		{"n not power of two", kdfParams{N: 3, R: 1, P: 1, DKLen: scryptDKLen}, false},
		{"n negative", kdfParams{N: -2, R: 1, P: 1, DKLen: scryptDKLen}, false},
		{"n huge power of two", kdfParams{N: 1 << 40, R: 1, P: 1, DKLen: scryptDKLen}, false},
		{"n max int", kdfParams{N: int(^uint(0) >> 1), R: 1, P: 1, DKLen: scryptDKLen}, false},
		{"r zero", kdfParams{N: 2, R: 0, P: 1, DKLen: scryptDKLen}, false},
		{"r huge", kdfParams{N: 2, R: 1 << 40, P: 1, DKLen: scryptDKLen}, false},
		{"p zero", kdfParams{N: 2, R: 1, P: 0, DKLen: scryptDKLen}, false},
		{"p huge", kdfParams{N: 2, R: 1, P: maxScryptP + 1, DKLen: scryptDKLen}, false},
		{"n and r exceed memory budget", kdfParams{N: 1 << 20, R: 1 << 10, P: 1, DKLen: scryptDKLen}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.param.validate()
			if tc.valid && err != nil {
				t.Fatalf("want valid, got %v", err)
			}
			if !tc.valid && err == nil {
				t.Fatal("want error, got nil")
			}
		})
	}
}

// TestEncryptDecryptKeyRoundTrip asserts that the parameters written by
// encryptKey pass validation and decrypt back to the same key.
func TestEncryptDecryptKeyRoundTrip(t *testing.T) {
	t.Parallel()

	const password = "some-password"

	k, err := crypto.EDGSecp256_K1.Generate()
	if err != nil {
		t.Fatal(err)
	}
	blob, err := encryptKey(k, password, crypto.EDGSecp256_K1)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decryptKey(blob, password, crypto.EDGSecp256_K1)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(k) {
		t.Fatal("decrypted key does not match the encrypted one")
	}
}

// TestDecryptKeyInvalidKDFParams asserts that a keyfile whose scrypt parameters
// omit the derived key length is rejected before scrypt.Key is called, rather
// than deriving a key of unusable length.
func TestDecryptKeyInvalidKDFParams(t *testing.T) {
	t.Parallel()

	blob := []byte(`{"CrYpto":{"Cipher":"aes-128-ctr","kdf":"scrypt","kdfpArAms":{"n":2,"r":1,"p":1}},"version":3}`)

	if _, err := decryptKey(blob, "some-password", crypto.EDGSecp256_K1); err == nil {
		t.Fatal("expected an error for a keyfile with invalid scrypt parameters")
	}
}
