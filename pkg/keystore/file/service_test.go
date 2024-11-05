// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file_test

import (
	"bytes"
	"crypto/elliptic"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/keystore/file"
	"github.com/ethersphere/bee/v2/pkg/keystore/test"
)

func TestService(t *testing.T) {
	t.Parallel()

	t.Run("EDGSecp256_K1", func(t *testing.T) {
		test.Service(t, file.New(t.TempDir()), crypto.EDGSecp256_K1)
	})

	t.Run("EDGSecp256_R1", func(t *testing.T) {
		test.Service(t, file.New(t.TempDir()), crypto.EDGSecp256_R1)
	})
}

func TestDeprecatedEllipticMarshal(t *testing.T) {
	t.Parallel()

	t.Run("EDGSecp256_K1", func(t *testing.T) {
		pk, err := crypto.EDGSecp256_K1.Generate()
		if err != nil {
			t.Fatal(err)
		}

		pubBytes := ethcrypto.S256().Marshal(pk.X, pk.Y)
		if len(pubBytes) != 65 {
			t.Fatalf("public key bytes length mismatch")
		}

		// nolint:staticcheck
		pubBytesDeprecated := elliptic.Marshal(btcec.S256(), pk.X, pk.Y)

		if !bytes.Equal(pubBytes, pubBytesDeprecated) {
			t.Fatalf("public key bytes mismatch")
		}
	})

	t.Run("EDGSecp256_R1", func(t *testing.T) {
		pk, err := crypto.EDGSecp256_R1.Generate()
		if err != nil {
			t.Fatal(err)
		}

		pkECDH, err := pk.ECDH()
		if err != nil {
			t.Fatalf("ecdh failed: %v", err)
		}

		pubBytes := pkECDH.PublicKey().Bytes()
		if len(pubBytes) != 65 {
			t.Fatalf("public key bytes length mismatch")
		}

		// nolint:staticcheck
		pubBytesDeprecated := elliptic.Marshal(elliptic.P256(), pk.X, pk.Y)

		if !bytes.Equal(pubBytes, pubBytesDeprecated) {
			t.Fatalf("public key bytes mismatch")
		}
	})
}
