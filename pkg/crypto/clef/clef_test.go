// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package clef_test

import (
	"bytes"
	"crypto/ecdsa"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/crypto/clef"
)

type mockClef struct {
	accounts  []accounts.Account
	signature []byte

	signedMimeType string
	signedData     []byte
	signedAccount  accounts.Account
}

func (m *mockClef) SignData(account accounts.Account, mimeType string, data []byte) ([]byte, error) {
	m.signedAccount = account
	m.signedMimeType = mimeType
	m.signedData = data
	return m.signature, nil
}

func (m *mockClef) Accounts() []accounts.Account {
	return m.accounts
}

func (m *mockClef) SignTx(account accounts.Account, transaction *types.Transaction, chainId *big.Int) (*types.Transaction, error) {
	return nil, nil
}

func TestNewClefSigner(t *testing.T) {
	ethAddress := common.HexToAddress("0x31415b599f636129AD03c196cef9f8f8b184D5C7")
	testSignature := make([]byte, 65)

	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	publicKey := &key.PublicKey

	mock := &mockClef{
		accounts: []accounts.Account{
			{
				Address: ethAddress,
			},
		},
		signature: testSignature,
	}

	signer, err := clef.NewSigner(mock, func(signature, data []byte) (*ecdsa.PublicKey, error) {
		if !bytes.Equal(testSignature, signature) {
			t.Fatalf("wrong data used for recover. expected %v got %v", testSignature, signature)
		}

		if !bytes.Equal(clef.ClefRecoveryMessage, data) {
			t.Fatalf("wrong data used for recover. expected %v got %v", clef.ClefRecoveryMessage, data)
		}
		return publicKey, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if mock.signedAccount.Address != ethAddress {
		t.Fatalf("wrong account used for signing. expected %v got %v", ethAddress, mock.signedAccount.Address)
	}

	if mock.signedMimeType != accounts.MimetypeTextPlain {
		t.Fatalf("wrong mime type used for signing. expected %v got %v", accounts.MimetypeTextPlain, mock.signedMimeType)
	}

	if !bytes.Equal(mock.signedData, clef.ClefRecoveryMessage) {
		t.Fatalf("wrong data used for signing. expected %v got %v", clef.ClefRecoveryMessage, mock.signedData)
	}

	signerPublicKey, err := signer.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	if signerPublicKey != publicKey {
		t.Fatalf("wrong public key. expected %v got %v", publicKey, signerPublicKey)
	}
}

func TestClefNoAccounts(t *testing.T) {
	mock := &mockClef{
		accounts: []accounts.Account{},
	}

	_, err := clef.NewSigner(mock, nil)
	if err == nil {
		t.Fatal("expected ErrNoAccounts error if no accounts")
	}
	if !errors.Is(err, clef.ErrNoAccounts) {
		t.Fatalf("expected ErrNoAccounts error but got %v", err)
	}
}
