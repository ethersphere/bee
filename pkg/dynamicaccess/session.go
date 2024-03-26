package dynamicaccess

import (
	"crypto/ecdsa"
	"errors"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/keystore"
)

// Session represents an interface for a Diffie-Helmann key derivation
type Session interface {
	// Key returns a derived key for each nonce
	Key(publicKey *ecdsa.PublicKey, nonces [][]byte) ([][]byte, error)
}

var _ Session = (*session)(nil)

type session struct {
	key *ecdsa.PrivateKey
}

func (s *session) Key(publicKey *ecdsa.PublicKey, nonces [][]byte) ([][]byte, error) {
	if publicKey == nil {
		return nil, errors.New("invalid public key")
	}
	x, y := publicKey.Curve.ScalarMult(publicKey.X, publicKey.Y, s.key.D.Bytes())
	if x == nil || y == nil {
		return nil, errors.New("shared secret is point at infinity")
	}

	if len(nonces) == 0 {
		return [][]byte{(*x).Bytes()}, nil
	}

	keys := make([][]byte, 0, len(nonces))
	for _, nonce := range nonces {
		key, err := crypto.LegacyKeccak256(append(x.Bytes(), nonce...))
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}

	return keys, nil
}

func NewDefaultSession(key *ecdsa.PrivateKey) Session {
	return &session{
		key: key,
	}
}

// Currently implemented only in mock/session.go
func NewFromKeystore(ks keystore.Service, tag, password string) Session {
	return nil
}
