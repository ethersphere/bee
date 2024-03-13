package mock

import (
	"crypto/ecdsa"
)

type DiffieHellmanMock struct {
	SharedSecretFunc func(publicKey *ecdsa.PublicKey, tag string, salt []byte) ([]byte, error)
	key              *ecdsa.PrivateKey
}

func (dhm *DiffieHellmanMock) SharedSecret(publicKey *ecdsa.PublicKey, tag string, salt []byte) ([]byte, error) {
	if dhm.SharedSecretFunc == nil {
		return nil, nil
	}
	return dhm.SharedSecretFunc(publicKey, tag, salt)

}

func NewDiffieHellmanMock(key *ecdsa.PrivateKey) *DiffieHellmanMock {
	return &DiffieHellmanMock{key: key}
}
