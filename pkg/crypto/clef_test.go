package crypto_test

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethersphere/bee/pkg/crypto"
)

type mockClef struct {
	accounts []accounts.Account
}

func (m *mockClef) SignData(account accounts.Account, mimeType string, data []byte) ([]byte, error) {
	return nil, nil
}

func (m *mockClef) Accounts() []accounts.Account {
	return m.accounts
}

func TestClefNoAccounts(t *testing.T) {
	clef := &mockClef{
		accounts: []accounts.Account{},
	}

	_, err := crypto.NewClefSigner(clef)
	if err == nil {
		t.Fatal("expected ErrNoAccounts error if no accounts")
	}
	if !errors.Is(err, crypto.ErrNoAccounts) {
		t.Fatalf("expected ErrNoAccounts error but got %v", err)
	}
}
