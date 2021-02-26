// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/crypto/eip712"
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

	chainID := big.NewInt(10)

	tx, err := signer.SignTx(types.NewTransaction(0, beneficiary, big.NewInt(0), 21000, big.NewInt(1), []byte{1}), chainID)
	if err != nil {
		t.Fatal(err)
	}

	expectedR := math.MustParseBig256("0x2937d18005a8236330b95c6b271ff46b06d5bf25355a06ff50939c9023245a99")
	expectedS := math.MustParseBig256("0xcd7c13b2bb88a3d99004a80898fc05b50263f445f6c53ef7baf7ffca0e4a1bf")
	expectedV := math.MustParseBig256("0x37")

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

var testTypedData = &eip712.TypedData{
	Domain: eip712.TypedDataDomain{
		Name:    "test",
		Version: "1.0",
	},
	Types: eip712.Types{
		"EIP712Domain": {
			{
				Name: "name",
				Type: "string",
			},
			{
				Name: "version",
				Type: "string",
			},
		},
		"MyType": {
			{
				Name: "test",
				Type: "string",
			},
		},
	},
	Message: eip712.TypedDataMessage{
		"test": "abc",
	},
	PrimaryType: "MyType",
}

func TestDefaultSignerTypedData(t *testing.T) {
	data, err := hex.DecodeString("634fb5a872396d9693e5c9f9d7233cfa93f395c093371017ff44aa9ae6564cdd")
	if err != nil {
		t.Fatal(err)
	}

	privKey, err := crypto.DecodeSecp256k1PrivateKey(data)
	if err != nil {
		t.Fatal(err)
	}

	signer := crypto.NewDefaultSigner(privKey)

	sig, err := signer.SignTypedData(testTypedData)
	if err != nil {
		t.Fatal(err)
	}

	expected, err := hex.DecodeString("60f054c45d37a0359d4935da0454bc19f02a8c01ceee8a112cfe48c8e2357b842e897f76389fb96947c6d2c80cbfe081052204e7b0c3cc1194a973a09b1614f71c")
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(expected, sig) {
		t.Fatalf("wrong signature. expected %x, got %x", expected, sig)
	}
}

func TestRecoverEIP712(t *testing.T) {
	data, err := hex.DecodeString("634fb5a872396d9693e5c9f9d7233cfa93f395c093371017ff44aa9ae6564cdd")
	if err != nil {
		t.Fatal(err)
	}

	privKey, err := crypto.DecodeSecp256k1PrivateKey(data)
	if err != nil {
		t.Fatal(err)
	}

	expected, err := hex.DecodeString("60f054c45d37a0359d4935da0454bc19f02a8c01ceee8a112cfe48c8e2357b842e897f76389fb96947c6d2c80cbfe081052204e7b0c3cc1194a973a09b1614f71c")
	if err != nil {
		t.Fatal(err)
	}

	pubKey, err := crypto.RecoverEIP712(expected, testTypedData)
	if err != nil {
		t.Fatal(err)
	}

	if privKey.PublicKey.X.Cmp(pubKey.X) != 0 {
		t.Fatalf("recovered wrong public key. wanted %x, got %x", privKey.PublicKey, pubKey)
	}

	if privKey.PublicKey.Y.Cmp(pubKey.Y) != 0 {
		t.Fatalf("recovered wrong public key. wanted %x, got %x", privKey.PublicKey, pubKey)
	}
}

func TestDefaultSignerDeterministic(t *testing.T) {
	data, err := hex.DecodeString("634fb5a872396d9693e5c9f9d7233cfa93f395c093371017ff44aa9ae6564cdd")
	if err != nil {
		t.Fatal(err)
	}

	privKey, err := crypto.DecodeSecp256k1PrivateKey(data)
	if err != nil {
		t.Fatal(err)
	}

	digest, err := hex.DecodeString("2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae")
	if err != nil {
		t.Fatal(err)
	}

	signer := crypto.NewDefaultSigner(privKey)
	sig, err := signer.Sign(digest)
	if err != nil {
		t.Fatal(err)
	}
	expSig, err := hex.DecodeString("336d24afef78c5883b96ad9a62552a8db3d236105cb059ddd04dc49680869dc16234f6852c277087f025d4114c4fac6b40295ecffd1194a84cdb91bd571769491b")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expSig, sig) {
		t.Fatal("signature mismatch")
	}
}
