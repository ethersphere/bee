package dynamicaccess

import (
	"context"
	"crypto/ecdsa"
	"errors"

	encryption "github.com/ethersphere/bee/pkg/encryption"
	file "github.com/ethersphere/bee/pkg/file"
	manifest "github.com/ethersphere/bee/pkg/manifest"
	"golang.org/x/crypto/sha3"
)

var hashFunc = sha3.NewLegacyKeccak256

type AccessLogic interface {
	Get(act_root_hash string, encryped_ref string, publisher string, tag string) (string, error)
	GetLookUpKey(publisher string, tag string) (string, error)
	GetAccessKeyDecriptionKey(publisher string, tag string) (string, error)
	GetEncryptedAccessKey(act_root_hash string, lookup_key string) (manifest.Entry, error)
}

type DefaultAccessLogic struct {
	diffieHellman DiffieHellman
	//encryption    encryption.Interface
	act defaultAct
}

// Will give back Swarm reference with symmertic encryption key (128 byte)
// @publisher: public key
func (al *DefaultAccessLogic) GetLookUpKey(publisher string, tag string) (string, error) {
	zeroByteArray := []byte{0}
	// Generate lookup key using Diffie Hellman
	lookup_key, err := al.diffieHellman.SharedSecret(publisher, tag, zeroByteArray)
	if err != nil {
		return "", err
	}
	return lookup_key, nil

}

func (al *DefaultAccessLogic) GetAccessKeyDecriptionKey(publisher string, tag string) (string, error) {
	oneByteArray := []byte{1}
	// Generate access key decryption key using Diffie Hellman
	access_key_decryption_key, err := al.diffieHellman.SharedSecret(publisher, tag, oneByteArray)
	if err != nil {
		return "", err
	}
	return access_key_decryption_key, nil
}

func (al *DefaultAccessLogic) GetEncryptedAccessKey(act_root_hash string, lookup_key string) (manifest.Entry, error) {
	if act_root_hash == "" {
		return nil, errors.New("no ACT root hash was provided")
	}
	if lookup_key == "" {
		return nil, errors.New("no lookup key")
	}

	manifest_raw, err := al.act.Get(act_root_hash)
	if err != nil {
		return nil, err
	}
	al.act.Get(act_root_hash)

	// Lookup encrypted access key from the ACT manifest
	var loadSaver file.LoadSaver
	var ctx context.Context
	loadSaver.Load(ctx, []byte(manifest_raw)) // Load the manifest file into loadSaver
	//y, err := x.Load(ctx, []byte(manifest_obj))
	manifestObj, err := manifest.NewDefaultManifest(loadSaver, false)
	if err != nil {
		return nil, err
	}
	encrypted_access_key, err := manifestObj.Lookup(ctx, lookup_key)
	if err != nil {
		return nil, err
	}

	return encrypted_access_key, nil
}

func (al *DefaultAccessLogic) Get(act_root_hash string, encryped_ref string, publisher string, tag string) (string, error) {

	lookup_key, err := al.GetLookUpKey(publisher, tag)
	if err != nil {
		return "", err
	}
	access_key_decryption_key, err := al.GetAccessKeyDecriptionKey(publisher, tag)
	if err != nil {
		return "", err
	}

	// Lookup encrypted access key from the ACT manifest

	encrypted_access_key, err := al.GetEncryptedAccessKey(act_root_hash, lookup_key)
	if err != nil {
		return "", err
	}

	// Decrypt access key
	access_key_cipher := encryption.New(encryption.Key(access_key_decryption_key), 4096, uint32(0), hashFunc)
	access_key, err := access_key_cipher.Decrypt(encrypted_access_key.Reference().Bytes())
	if err != nil {
		return "", err
	}

	// Decrypt reference
	ref_cipher := encryption.New(access_key, 4096, uint32(0), hashFunc)
	ref, err := ref_cipher.Decrypt([]byte(encryped_ref))
	if err != nil {
		return "", err
	}

	return string(ref), nil
}

func NewAccessLogic(diffieHellmanPrivateKey *ecdsa.PrivateKey) AccessLogic {
	return &DefaultAccessLogic{
		diffieHellman: NewDiffieHellman(diffieHellmanPrivateKey),
		//encryption:    encryption.New(key, padding, initCtr, hashFunc),
		act: defaultAct{},

		// {
		// 	AddFunc: func(ref string, publisher string, tag string) error {
		// 		return nil
		// 	},
		// 	GetFunc: func(ref string, publisher string, tag string) (string, error) {
		// 		return "", nil
		// 	},
		// },
	}
}

// -------
// act: &mock.ContainerMock{
// 	AddFunc: func(ref string, publisher string, tag string) error {
// 		return nil
// 	},
// 	GetFunc: func(ref string, publisher string, tag string) (string, error) {
// 		return "", nil
// 	},
// },
