// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/crypto"
)

func TestDefaultSigner(t *testing.T) {
	testBytes := []byte("test string")
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal()
	}

	signer := crypto.NewDefaultSigner(privKey)
	signature, err := signer.Sign(testBytes)
	if err != nil {
		t.Fatal()
	}

	t.Run("sign & recover with valid signature", func(t *testing.T) {
		pubKey, err := signer.Recover(signature, testBytes)
		if err != nil {
			t.Fatal()
		}

		if pubKey.X.Cmp(privKey.PublicKey.X) != 0 || pubKey.Y.Cmp(privKey.PublicKey.Y) != 0 {
			t.Fatalf("wanted %v but got %v", pubKey, &privKey.PublicKey)
		}
	})

	t.Run("sign & recover with invalid signature", func(t *testing.T) {
		pubKey, err := signer.Recover(signature, testBytes)
		if err != nil {
			t.Fatal()
		}

		otherPrivKey, err := crypto.GenerateSecp256k1Key()
		if err != nil {
			t.Fatal()
		}

		if pubKey.X.Cmp(otherPrivKey.PublicKey.X) == 0 && pubKey.Y.Cmp(otherPrivKey.PublicKey.Y) == 0 {
			t.Fatal("shold have been different")
		}
	})
}
