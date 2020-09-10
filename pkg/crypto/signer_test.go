// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto_test

import (
	"errors"
	"testing"

	"github.com/ethersphere/bee/pkg/crypto"
)

func TestDefaultSigner(t *testing.T) {
	testBytes := []byte("test string")
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	signer := crypto.NewDefaultSigner(privKey)
	signature, err := signer.Sign(testBytes)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("OK - sign & recover", func(t *testing.T) {
		pubKey, err := crypto.Recover(signature, testBytes)
		if err != nil {
			t.Fatal(err)
		}

		if pubKey.X.Cmp(privKey.PublicKey.X) != 0 || pubKey.Y.Cmp(privKey.PublicKey.Y) != 0 {
			t.Fatalf("wanted %v but got %v", pubKey, &privKey.PublicKey)
		}
	})

	t.Run("OK - recover with invalid data", func(t *testing.T) {
		pubKey, err := crypto.Recover(signature, []byte("invalid"))
		if err != nil {
			t.Fatal(err)
		}

		if pubKey.X.Cmp(privKey.PublicKey.X) == 0 && pubKey.Y.Cmp(privKey.PublicKey.Y) == 0 {
			t.Fatal("expected different public key")
		}
	})

	t.Run("OK - recover with short signature", func(t *testing.T) {
		_, err := crypto.Recover([]byte("invalid"), testBytes)
		if err == nil {
			t.Fatal("expected invalid length error but got none")
		}
		if !errors.Is(err, crypto.ErrInvalidLength) {
			t.Fatalf("expected invalid length error but got %v", err)
		}
	})
}
