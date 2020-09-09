package crypto_test

import (
	"bytes"
	"crypto/ecdsa"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/crypto"
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

func TestNewClefSigner(t *testing.T) {
	ethAddress := common.HexToAddress("0x31415b599f636129AD03c196cef9f8f8b184D5C7")
	testSignature := make([]byte, 65)

	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	publicKey := &key.PublicKey

	clef := &mockClef{
		accounts: []accounts.Account{
			{
				Address: ethAddress,
			},
		},
		signature: testSignature,
	}

	signer, err := crypto.NewClefSigner(clef, func(signature, data []byte) (*ecdsa.PublicKey, error) {
		if !bytes.Equal(testSignature, signature) {
			t.Fatalf("wrong data used for recover. expected %v got %v", testSignature, signature)
		}

		if !bytes.Equal(crypto.ClefRecoveryMessage, data) {
			t.Fatalf("wrong data used for recover. expected %v got %v", crypto.ClefRecoveryMessage, data)
		}
		return publicKey, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if clef.signedAccount.Address != ethAddress {
		t.Fatalf("wrong account used for signing. expected %v got %v", ethAddress, clef.signedAccount.Address)
	}

	if clef.signedMimeType != accounts.MimetypeTextPlain {
		t.Fatalf("wrong mime type used for signing. expected %v got %v", accounts.MimetypeTextPlain, clef.signedMimeType)
	}

	if !bytes.Equal(clef.signedData, crypto.ClefRecoveryMessage) {
		t.Fatalf("wrong data used for signing. expected %v got %v", crypto.ClefRecoveryMessage, clef.signedData)
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
	clef := &mockClef{
		accounts: []accounts.Account{},
	}

	_, err := crypto.NewClefSigner(clef, nil)
	if err == nil {
		t.Fatal("expected ErrNoAccounts error if no accounts")
	}
	if !errors.Is(err, crypto.ErrNoAccounts) {
		t.Fatalf("expected ErrNoAccounts error but got %v", err)
	}
}
