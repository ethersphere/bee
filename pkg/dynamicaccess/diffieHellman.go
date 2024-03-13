package dynamicaccess

import (
	"crypto/ecdsa"
	"errors"

	"github.com/ethersphere/bee/pkg/crypto"
)

type DiffieHellman interface {
	SharedSecret(publicKey *ecdsa.PublicKey, tag string, moment []byte) ([]byte, error) // tag- topic?
}

var _ DiffieHellman = (*defaultDiffieHellman)(nil)

type defaultDiffieHellman struct {
	key *ecdsa.PrivateKey
}

func (dh *defaultDiffieHellman) SharedSecret(publicKey *ecdsa.PublicKey, tag string, salt []byte) ([]byte, error) {
	x, _ := publicKey.Curve.ScalarMult(publicKey.X, publicKey.Y, dh.key.D.Bytes())
	if x == nil {
		return nil, errors.New("shared secret is point at infinity")
	}
	return crypto.LegacyKeccak256(append(x.Bytes(), salt...))
}

func NewDiffieHellman(key *ecdsa.PrivateKey) DiffieHellman {
	return &defaultDiffieHellman{key: key}

}
