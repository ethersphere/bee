// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package elgamal_test

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/encryption/elgamal"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestElgamalCorrect(t *testing.T) {
	plaintext := []byte("some highly confidential text")
	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	pub := &key.PublicKey
	salt := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		t.Fatal(err)
	}
	padding := 4032
	enc, ephpub, err := elgamal.NewEncryptor(pub, salt, padding, swarm.NewHasher)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if len(ciphertext) != padding {
		t.Fatalf("ciphertext has incorrect length: expected %v,  got %v", padding, len(ciphertext))
	}

	dec, err := elgamal.NewDecrypter(key, ephpub, salt, swarm.NewHasher)
	if err != nil {
		t.Fatal(err)
	}
	expected := plaintext
	decryptedtext, err := dec.Decrypt(ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if len(decryptedtext) != padding {
		t.Fatalf("decrypted text has incorrect length: expected %v,  got %v", padding, len(decryptedtext))
	}
	plaintext = decryptedtext[:len(expected)]
	if !bytes.Equal(plaintext, expected) {
		t.Fatalf("original and encrypted-decrypted plaintexts do no match: expected %x, got %x", expected, plaintext)
	}

}
