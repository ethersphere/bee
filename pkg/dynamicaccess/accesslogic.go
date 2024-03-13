package dynamicaccess

import (
	"context"
	"crypto/ecdsa"
	"errors"

	encryption "github.com/ethersphere/bee/pkg/encryption"
	file "github.com/ethersphere/bee/pkg/file"
	manifest "github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/swarm"
	"golang.org/x/crypto/sha3"
)

var hashFunc = sha3.NewLegacyKeccak256

type AccessLogic interface {
	Get(act *Act, encryped_ref swarm.Address, publisher ecdsa.PublicKey, tag string) (string, error)
	//Add(act *Act, ref string, publisher ecdsa.PublicKey, tag string) (string, error)
	getLookUpKey(publisher ecdsa.PublicKey, tag string) (string, error)
	getAccessKeyDecriptionKey(publisher ecdsa.PublicKey, tag string) (string, error)
	getEncryptedAccessKey(act Act, lookup_key string) (manifest.Entry, error)
	//createEncryptedAccessKey(ref string)
	Add_New_Grantee_To_Content(act *Act, encryptedRef swarm.Address, publisherPubKey ecdsa.PublicKey, granteePubKey ecdsa.PublicKey) (*Act, error)
	ActInit(ref swarm.Address, publisher ecdsa.PublicKey, tag string) (*Act, swarm.Address, error)
	// CreateAccessKey()
}

type DefaultAccessLogic struct {
	diffieHellman DiffieHellman
	//encryption    encryption.Interface
	act defaultAct
}

// Will create a new Act list with only one element (the creator), and will also create encrypted_ref
func (al *DefaultAccessLogic) ActInit(ref swarm.Address, publisher ecdsa.PublicKey, tag string) (*Act, swarm.Address, error) {
	act := NewDefaultAct()

	lookup_key, _ := al.getLookUpKey(publisher, "")
	access_key_encryption_key, _ := al.getAccessKeyDecriptionKey(publisher, "")

	access_key_cipher := encryption.New(encryption.Key(access_key_encryption_key), 0, uint32(0), hashFunc)
	access_key := encryption.GenerateRandomKey(encryption.KeyLength)
	encrypted_access_key, _ := access_key_cipher.Encrypt([]byte(access_key))

	ref_cipher := encryption.New(access_key, 0, uint32(0), hashFunc)
	encrypted_ref, _ := ref_cipher.Encrypt(ref.Bytes())

	act.Add([]byte(lookup_key), encrypted_access_key)

	return &act, swarm.NewAddress(encrypted_ref), nil
}

// publisher is public key
func (al *DefaultAccessLogic) Add_New_Grantee_To_Content(act *Act, encryptedRef swarm.Address, publisherPubKey ecdsa.PublicKey, granteePubKey ecdsa.PublicKey) (*Act, error) {

	// error handling no encrypted_ref

	// 2 Diffie-Hellman for the publisher (the Creator)
	publisher_lookup_key, _ := al.getLookUpKey(publisherPubKey, "")
	publisher_ak_decryption_key, _ := al.getAccessKeyDecriptionKey(publisherPubKey, "")

	// Get previously generated access key
	access_key_decryption_cipher := encryption.New(encryption.Key(publisher_ak_decryption_key), 0, uint32(0), hashFunc)
	encrypted_ak, _ := al.getEncryptedAccessKey(*act, publisher_lookup_key)
	access_key, _ := access_key_decryption_cipher.Decrypt(encrypted_ak.Reference().Bytes())

	// --Encrypt access key for new Grantee--

	// 2 Diffie-Hellman for the Grantee
	lookup_key, _ := al.getLookUpKey(granteePubKey, "")
	access_key_encryption_key, _ := al.getAccessKeyDecriptionKey(granteePubKey, "")

	// Encrypt the access key for the new Grantee
	cipher := encryption.New(encryption.Key(access_key_encryption_key), 0, uint32(0), hashFunc)
	granteeEncryptedAccessKey, _ := cipher.Encrypt(access_key)
	// Add the new encrypted access key for the Act
	actObj := *act
	actObj.Add([]byte(lookup_key), granteeEncryptedAccessKey)

	return &actObj, nil

}

//
// act[lookupKey] := valamilyen_cipher.Encrypt(access_key)

// end of pseudo code like code

// func (al *DefaultAccessLogic) CreateAccessKey(reference string) {
// }

func (al *DefaultAccessLogic) getLookUpKey(publisher ecdsa.PublicKey, tag string) (string, error) {
	zeroByteArray := []byte{0}
	// Generate lookup key using Diffie Hellman
	lookup_key, err := al.diffieHellman.SharedSecret(&publisher, tag, zeroByteArray)
	if err != nil {
		return "", err
	}
	return string(lookup_key), nil

}

func (al *DefaultAccessLogic) getAccessKeyDecriptionKey(publisher ecdsa.PublicKey, tag string) (string, error) {
	oneByteArray := []byte{1}
	// Generate access key decryption key using Diffie Hellman
	access_key_decryption_key, err := al.diffieHellman.SharedSecret(&publisher, tag, oneByteArray)
	if err != nil {
		return "", err
	}
	return string(access_key_decryption_key), nil
}

func (al *DefaultAccessLogic) getEncryptedAccessKey(act Act, lookup_key string) (manifest.Entry, error) {
	if act == nil {
		return nil, errors.New("no ACT root hash was provided")
	}
	if lookup_key == "" {
		return nil, errors.New("no lookup key")
	}

	manifest_raw := act.Get([]byte(lookup_key))
	//al.act.Get(act_root_hash)

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

func (al *DefaultAccessLogic) Get(act *Act, encryped_ref swarm.Address, publisher ecdsa.PublicKey, tag string) (string, error) {

	lookup_key, err := al.getLookUpKey(publisher, tag)
	if err != nil {
		return "", err
	}
	access_key_decryption_key, err := al.getAccessKeyDecriptionKey(publisher, tag)
	if err != nil {
		return "", err
	}

	// Lookup encrypted access key from the ACT manifest

	encrypted_access_key, err := al.getEncryptedAccessKey(*act, lookup_key)
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
	ref, err := ref_cipher.Decrypt(encryped_ref.Bytes())
	if err != nil {
		return "", err
	}

	return string(ref), nil
}

func NewAccessLogic(diffieHellman DiffieHellman) AccessLogic {
	return &DefaultAccessLogic{
		diffieHellman: diffieHellman,
		act:           defaultAct{},
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
