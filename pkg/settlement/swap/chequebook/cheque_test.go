// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook_test

import (
	"bytes"

	"encoding/hex"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/crypto/eip712"
	signermock "github.com/ethersphere/bee/pkg/crypto/mock"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
)

func TestSignCheque(t *testing.T) {
	chequebookAddress := common.HexToAddress("0x8d3766440f0d7b949a5e32995d09619a7f86e632")
	beneficiaryAddress := common.HexToAddress("0xb8d424e9662fe0837fb1d728f1ac97cebb1085fe")
	signature := common.Hex2Bytes("abcd")
	cumulativePayout := big.NewInt(10)
	chainId := int64(1)
	cheque := &chequebook.Cheque{
		Chequebook:       chequebookAddress,
		Beneficiary:      beneficiaryAddress,
		CumulativePayout: cumulativePayout,
	}

	signer := signermock.New(
		signermock.WithSignTypedDataFunc(func(data *eip712.TypedData) ([]byte, error) {

			if data.Message["beneficiary"].(string) != beneficiaryAddress.Hex() {
				t.Fatal("signing cheque with wrong beneficiary")
			}

			if data.Message["chequebook"].(string) != chequebookAddress.Hex() {
				t.Fatal("signing cheque for wrong chequebook")
			}

			if data.Message["cumulativePayout"].(string) != cumulativePayout.String() {
				t.Fatal("signing cheque with wrong cumulativePayout")
			}

			return signature, nil
		}),
	)

	chequeSigner := chequebook.NewChequeSigner(signer, chainId)

	result, err := chequeSigner.Sign(cheque)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(result, signature) {
		t.Fatalf("returned wrong signature. wanted %x, got %x", signature, result)
	}
}

func TestSignChequeIntegration(t *testing.T) {
	chequebookAddress := common.HexToAddress("0xfa02D396842E6e1D319E8E3D4D870338F791AA25")
	beneficiaryAddress := common.HexToAddress("0x98E6C644aFeB94BBfB9FF60EB26fc9D83BBEcA79")
	cumulativePayout := big.NewInt(500)
	chainId := int64(1)

	data, err := hex.DecodeString("634fb5a872396d9693e5c9f9d7233cfa93f395c093371017ff44aa9ae6564cdd")
	if err != nil {
		t.Fatal(err)
	}

	privKey, err := crypto.DecodeSecp256k1PrivateKey(data)
	if err != nil {
		t.Fatal(err)
	}

	signer := crypto.NewDefaultSigner(privKey)

	cheque := &chequebook.Cheque{
		Chequebook:       chequebookAddress,
		Beneficiary:      beneficiaryAddress,
		CumulativePayout: cumulativePayout,
	}

	chequeSigner := chequebook.NewChequeSigner(signer, chainId)

	result, err := chequeSigner.Sign(cheque)
	if err != nil {
		t.Fatal(err)
	}

	// computed using ganache
	expectedSignature, err := hex.DecodeString("171b63fc598ae2c7987f4a756959dadddd84ccd2071e7b5c3aa3437357be47286125edc370c344a163ba7f4183dfd3611996274a13e4b3496610fc00c0e2fc421c")
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(result, expectedSignature) {
		t.Fatalf("returned wrong signature. wanted %x, got %x", expectedSignature, result)
	}
}
