package crypto

import (
	"crypto/ecdsa"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/external"
)

type clefSigner struct {
	clef    *external.ExternalSigner
	account accounts.Account // the account this signer will use
	pubKey  *ecdsa.PublicKey // the public key for the account
}

// defaultIpcPath returns the os-dependent default ipc path for clef
func defaultIpcPath() (string, error) {
	socket := "clef.ipc"
	// on windows clef uses top level pipes
	if runtime.GOOS == "windows" {
		return `\\.\pipe\` + socket, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	// on mac os clef defaults to ~/Library/Signer/clef.ipc
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Signer", socket), nil
	}

	// on unix clef defaults to ~/.clef/clef.ipc
	return filepath.Join(home, ".clef", socket), nil
}

// NewClefSigner creates a new connection to the signer at endpoint
// As clef does not expose public keys it signs a test message to recover the public key
func NewClefSigner(endpoint string) (signer Signer, err error) {
	if endpoint == "" {
		endpoint, err = defaultIpcPath()
		if err != nil {
			return nil, err
		}
	}

	clef, err := external.NewExternalSigner(endpoint)
	if err != nil {
		return nil, err
	}

	// get the list of available ethereum accounts
	clefAccounts := clef.Accounts()
	if len(clefAccounts) == 0 {
		return nil, errors.New("no accounts found in clef")
	}

	// pick the first account as the one we use
	account := clefAccounts[0]

	// clef currently does not expose the public key
	// sign some data so we can recover it
	recoveryMessage := []byte("public key recovery message")
	sig, err := clef.SignData(account, accounts.MimetypeTextPlain, recoveryMessage)
	if err != nil {
		return nil, err
	}

	pubKey, err := Recover(sig, recoveryMessage)
	if err != nil {
		return nil, err
	}

	return &clefSigner{
		clef:    clef,
		account: account,
		pubKey:  pubKey,
	}, nil
}

// PublicKey returns the public key recovered during creation
func (c *clefSigner) PublicKey() (*ecdsa.PublicKey, error) {
	return c.pubKey, nil
}

// SignData signs with the text/plain type which is the standard Ethereum prefix method
func (c *clefSigner) Sign(data []byte) ([]byte, error) {
	return c.clef.SignData(c.account, accounts.MimetypeTextPlain, data)
}
