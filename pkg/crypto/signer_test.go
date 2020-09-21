// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto_test

import (
	"encoding/hex"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/types"
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

func TestDefaultSignerEthereumAddress(t *testing.T) {
	data, err := hex.DecodeString("634fb5a872396d9693e5c9f9d7233cfa93f395c093371017ff44aa9ae6564cdd")
	if err != nil {
		t.Fatal(err)
	}

	privKey, err := crypto.DecodeSecp256k1PrivateKey(data)
	if err != nil {
		t.Fatal(err)
	}

	signer := crypto.NewDefaultSigner(privKey)
	ethAddress, err := signer.EthereumAddress()
	if err != nil {
		t.Fatal(err)
	}

	expected := common.HexToAddress("8d3766440f0d7b949a5e32995d09619a7f86e632")
	if ethAddress != expected {
		t.Fatalf("wrong signature. expected %x, got %x", expected, ethAddress)
	}
}

func TestDefaultSignerSignTx(t *testing.T) {
	data, err := hex.DecodeString("634fb5a872396d9693e5c9f9d7233cfa93f395c093371017ff44aa9ae6564cdd")
	if err != nil {
		t.Fatal(err)
	}

	privKey, err := crypto.DecodeSecp256k1PrivateKey(data)
	if err != nil {
		t.Fatal(err)
	}

	signer := crypto.NewDefaultSigner(privKey)
	beneficiary := common.HexToAddress("8d3766440f0d7b949a5e32995d09619a7f86e632")

	tx, err := signer.SignTx(types.NewTransaction(0, beneficiary, big.NewInt(0), 21000, big.NewInt(1), []byte{1}))
	if err != nil {
		t.Fatal(err)
	}

	expectedR := math.MustParseBig256("0x28815033e9b5b7ec32e40e3c90b6cd499c12de8a7da261fdad8b800c845b88ef")
	expectedS := math.MustParseBig256("0x71f1c08f754ee36e0c9743a2240d4b6640ea4d78c8dc2d83a599bdcf80ef9d5f")
	expectedV := math.MustParseBig256("0x1c")

	v, r, s := tx.RawSignatureValues()

	if expectedV.Cmp(v) != 0 {
		t.Fatalf("wrong v value. expected %x, got %x", expectedV, v)
	}

	if expectedR.Cmp(r) != 0 {
		t.Fatalf("wrong r value. expected %x, got %x", expectedR, r)
	}

	if expectedS.Cmp(s) != 0 {
		t.Fatalf("wrong s value. expected %x, got %x", expectedS, s)
	}
}
