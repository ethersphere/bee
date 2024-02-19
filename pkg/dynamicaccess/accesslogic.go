package dynamicaccess

import (
	"hash"

	"github.com/ethersphere/bee/pkg/dynamicaccess/mock"
	encryption "github.com/ethersphere/bee/pkg/encryption"
)

type AccessLogic interface {
	Get(encryped_ref string, publisher string, tag string) (string, error)
}

type DefaultAccessLogic struct {
	diffieHellman DiffieHellman
	encryption    encryption.Interface
	act           Act
}

func (al *DefaultAccessLogic) Get(encryped_ref string, publisher string, tag string) (string, error) {
	return "", nil
}

func NewAccessLogic(key encryption.Key, padding int, initCtr uint32, hashFunc func() hash.Hash) AccessLogic {
	return &DefaultAccessLogic{
		diffieHellman: &mock.DiffieHellmanMock{
			SharedSecretFunc: func(publicKey string, tag string, moment []byte) (string, error) {
				return publicKey, nil
			},
		},
		encryption: encryption.New(key, padding, initCtr, hashFunc),
		act: &mock.ContainerMock{
			AddFunc: func(ref string, publisher string, tag string) error {
				return nil
			},
			GetFunc: func(ref string, publisher string, tag string) (string, error) {
				return "", nil
			},
		},
	}
}
