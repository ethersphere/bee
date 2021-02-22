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
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/crypto/clef"
	"github.com/ethersphere/bee/pkg/crypto/eip712"
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
			{
				Address: common.Address{},
			},
		},
		signature: testSignature,
	}

	signer, err := clef.NewSigner(mock, nil, func(signature, data []byte) (*ecdsa.PublicKey, error) {
		if !bytes.Equal(testSignature, signature) {
			t.Fatalf("wrong data used for recover. expected %v got %v", testSignature, signature)
		}

		if !bytes.Equal(clef.ClefRecoveryMessage, data) {
			t.Fatalf("wrong data used for recover. expected %v got %v", clef.ClefRecoveryMessage, data)
		}
		return publicKey, nil
	}, nil)
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

func TestNewClefSignerSpecificAccount(t *testing.T) {
	ethAddress := common.HexToAddress("0x31415b599f636129AD03c196cef9f8f8b184D5C7")
	wantedAddress := common.HexToAddress("0x41415b599f636129AD03c196cef9f8f8b184D5C7")
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
			{
				Address: wantedAddress,
			},
		},
		signature: testSignature,
	}

	signer, err := clef.NewSigner(mock, nil, func(signature, data []byte) (*ecdsa.PublicKey, error) {
		if !bytes.Equal(testSignature, signature) {
			t.Fatalf("wrong data used for recover. expected %v got %v", testSignature, signature)
		}

		if !bytes.Equal(clef.ClefRecoveryMessage, data) {
			t.Fatalf("wrong data used for recover. expected %v got %v", clef.ClefRecoveryMessage, data)
		}
		return publicKey, nil
	}, &wantedAddress)
	if err != nil {
		t.Fatal(err)
	}

	if mock.signedAccount.Address != wantedAddress {
		t.Fatalf("wrong account used for signing. expected %v got %v", wantedAddress, mock.signedAccount.Address)
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

func TestNewClefSignerAccountUnavailable(t *testing.T) {
	ethAddress := common.HexToAddress("0x31415b599f636129AD03c196cef9f8f8b184D5C7")
	wantedAddress := common.HexToAddress("0x41415b599f636129AD03c196cef9f8f8b184D5C7")

	mock := &mockClef{
		accounts: []accounts.Account{
			{
				Address: ethAddress,
			},
		},
	}

	_, err := clef.NewSigner(mock, nil, func(signature, data []byte) (*ecdsa.PublicKey, error) {
		return nil, errors.New("called sign")
	}, &wantedAddress)
	if !errors.Is(err, clef.ErrAccountNotAvailable) {
		t.Fatalf("expected account to be not available. got error %v", err)
	}
}

func TestClefNoAccounts(t *testing.T) {
	mock := &mockClef{
		accounts: []accounts.Account{},
	}

	_, err := clef.NewSigner(mock, nil, nil, nil)
	if err == nil {
		t.Fatal("expected ErrNoAccounts error if no accounts")
	}
	if !errors.Is(err, clef.ErrNoAccounts) {
		t.Fatalf("expected ErrNoAccounts error but got %v", err)
	}
}

type mockRpc struct {
	call func(result interface{}, method string, args ...interface{}) error
}

func (m *mockRpc) Call(result interface{}, method string, args ...interface{}) error {
	return m.call(result, method, args...)
}

func TestClefTypedData(t *testing.T) {
	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	publicKey := &key.PublicKey
	signature := common.FromHex("0xabcdef")

	account := common.HexToAddress("21b26864067deb88e2d5cdca512167815f2910d3")

	typedData := &eip712.TypedData{
		PrimaryType: "MyType",
	}

	signer, err := clef.NewSigner(&mockClef{
		accounts: []accounts.Account{
			{
				Address: account,
			},
		},
		signature: make([]byte, 65),
	}, &mockRpc{
		call: func(result interface{}, method string, args ...interface{}) error {
			if method != "account_signTypedData" {
				t.Fatalf("called wrong method. was %s", method)
			}
			if args[0].(common.Address) != account {
				t.Fatalf("called with wrong account. was %x, wanted %x", args[0].(common.Address), account)
			}
			if args[1].(*eip712.TypedData) != typedData {
				t.Fatal("called with wrong data")
			}
			*result.(*hexutil.Bytes) = signature
			return nil
		},
	}, func(signature, data []byte) (*ecdsa.PublicKey, error) {
		return publicKey, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	s, err := signer.SignTypedData(typedData)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(s, signature) {
		t.Fatalf("wrong signature. wanted %x, got %x", signature, s)
	}
}
