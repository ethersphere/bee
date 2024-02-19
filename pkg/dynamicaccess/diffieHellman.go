package dynamicaccess

import (
	"crypto/ecdsa"

	Crypto "github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/keystore"
	KeyStoreMem "github.com/ethersphere/bee/pkg/keystore/mem"
)

type DiffieHellman interface {
	SharedSecret(pubKey, tag string, moment []byte) (string, error)
}

type defaultDiffieHellman struct {
	key             *ecdsa.PrivateKey
	keyStoreService keystore.Service
	keyStoreEdg     keystore.EDG
}

func (d *defaultDiffieHellman) SharedSecret(pubKey string, tag string, moment []byte) (string, error) {
	return "", nil
}

func NewDiffieHellman(key *ecdsa.PrivateKey) DiffieHellman {
	return &defaultDiffieHellman{
		key:             key,
		keyStoreService: KeyStoreMem.New(),
		keyStoreEdg:     Crypto.EDGSecp256_K1,
	}
}
