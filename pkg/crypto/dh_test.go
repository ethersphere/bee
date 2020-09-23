// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"io"
	"testing"

	"github.com/btcsuite/btcd/btcec"
	"github.com/ethersphere/bee/pkg/crypto"
)

func TestECDHCorrect(t *testing.T) {
	key0, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	dh0 := crypto.NewDH(key0)

	key1, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	dh1 := crypto.NewDH(key1)

	salt := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		t.Fatal(err)
	}

	sk0, err := dh0.SharedKey(&key1.PublicKey, salt)
	if err != nil {
		t.Fatal(err)
	}
	sk1, err := dh1.SharedKey(&key0.PublicKey, salt)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(sk0, sk1) {
		t.Fatal("shared secrets do not match")
	}
}

func TestSharedKey(t *testing.T) {
	data, err := hex.DecodeString("c786dd84b61485de12146fd9c4c02d87e8fd95f0542765cb7fc3d2e428c0bcfa")
	if err != nil {
		t.Fatal(err)
	}

	privKey, err := crypto.DecodeSecp256k1PrivateKey(data)
	if err != nil {
		t.Fatal(err)
	}
	data, err = hex.DecodeString("0271e574ad8f6a6c998c84c27df18124fddd906aba9d852150da4223edde14044f")
	if err != nil {
		t.Fatal(err)
	}
	pubkey, err := btcec.ParsePubKey(data, btcec.S256())
	if err != nil {
		t.Fatal(err)
	}
	salt, err := hex.DecodeString("cb7e692f211f8ae4f858ff56ce8a4fc0e40bae1a36f8283f0ceb6bb4be133f1e")
	if err != nil {
		t.Fatal(err)
	}

	dh := crypto.NewDH(privKey)
	sk, err := dh.SharedKey((*ecdsa.PublicKey)(pubkey), salt)
	if err != nil {
		t.Fatal(err)
	}

	expectedSKHex := "9edbd3beeb48c090158ccb82d679c5ea2bcb74850d34fe55c10b32e16b822007"
	expectedSK, err := hex.DecodeString(expectedSKHex)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(sk, expectedSK) {
		t.Fatalf("incorrect shared key: expected %v, got %x", expectedSK, sk)
	}

}
