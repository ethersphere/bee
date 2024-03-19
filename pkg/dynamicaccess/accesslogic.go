package dynamicaccess

import (
	"crypto/ecdsa"

	encryption "github.com/ethersphere/bee/pkg/encryption"
	"github.com/ethersphere/bee/pkg/swarm"
	"golang.org/x/crypto/sha3"
)

var hashFunc = sha3.NewLegacyKeccak256

type AccessLogic interface {
	Get(act Act, encryped_ref swarm.Address, publisher ecdsa.PublicKey, tag string) (swarm.Address, error)
	EncryptRef(act Act, publisherPubKey ecdsa.PublicKey, ref swarm.Address) (swarm.Address, error)
	//Add(act *Act, ref string, publisher ecdsa.PublicKey, tag string) (string, error)
	getLookUpKey(publisher ecdsa.PublicKey, tag string) ([]byte, error)
	getAccessKeyDecriptionKey(publisher ecdsa.PublicKey, tag string) ([]byte, error)
	getEncryptedAccessKey(act Act, lookup_key []byte) ([]byte, error)
	//createEncryptedAccessKey(ref string)
	Add_New_Grantee_To_Content(act Act, publisherPubKey, granteePubKey ecdsa.PublicKey) (Act, error)
	AddPublisher(act Act, publisher ecdsa.PublicKey, tag string) (Act, error)
	// CreateAccessKey()
}

type DefaultAccessLogic struct {
	diffieHellman DiffieHellman
	//encryption    encryption.Interface
}

// Will create a new Act list with only one element (the creator), and will also create encrypted_ref
func (al *DefaultAccessLogic) AddPublisher(act Act, publisher ecdsa.PublicKey, tag string) (Act, error) {
	access_key := encryption.GenerateRandomKey(encryption.KeyLength)

	lookup_key, _ := al.getLookUpKey(publisher, "")
	access_key_encryption_key, _ := al.getAccessKeyDecriptionKey(publisher, "")

	access_key_cipher := encryption.New(encryption.Key(access_key_encryption_key), 0, uint32(0), hashFunc)
	encrypted_access_key, _ := access_key_cipher.Encrypt(access_key)

	act.Add(lookup_key, encrypted_access_key)

	return act, nil
}

func (al *DefaultAccessLogic) EncryptRef(act Act, publisherPubKey ecdsa.PublicKey, ref swarm.Address) (swarm.Address, error) {
	access_key := al.getAccessKey(act, publisherPubKey)
	ref_cipher := encryption.New(access_key, 0, uint32(0), hashFunc)
	encrypted_ref, _ := ref_cipher.Encrypt(ref.Bytes())
	return swarm.NewAddress(encrypted_ref), nil
}

// publisher is public key
func (al *DefaultAccessLogic) Add_New_Grantee_To_Content(act Act, publisherPubKey, granteePubKey ecdsa.PublicKey) (Act, error) {

	// error handling no encrypted_ref

	// 2 Diffie-Hellman for the publisher (the Creator)
	// Get previously generated access key
	access_key := al.getAccessKey(act, publisherPubKey)

	// --Encrypt access key for new Grantee--

	// 2 Diffie-Hellman for the Grantee
	lookup_key, _ := al.getLookUpKey(granteePubKey, "")
	access_key_encryption_key, _ := al.getAccessKeyDecriptionKey(granteePubKey, "")

	// Encrypt the access key for the new Grantee
	cipher := encryption.New(encryption.Key(access_key_encryption_key), 0, uint32(0), hashFunc)
	granteeEncryptedAccessKey, _ := cipher.Encrypt(access_key)
	// Add the new encrypted access key for the Act
	act.Add(lookup_key, granteeEncryptedAccessKey)

	return act, nil

}

func (al *DefaultAccessLogic) getAccessKey(act Act, publisherPubKey ecdsa.PublicKey) []byte {
	publisher_lookup_key, _ := al.getLookUpKey(publisherPubKey, "")
	publisher_ak_decryption_key, _ := al.getAccessKeyDecriptionKey(publisherPubKey, "")

	access_key_decryption_cipher := encryption.New(encryption.Key(publisher_ak_decryption_key), 0, uint32(0), hashFunc)
	encrypted_ak, _ := al.getEncryptedAccessKey(act, publisher_lookup_key)
	access_key, _ := access_key_decryption_cipher.Decrypt(encrypted_ak)
	return access_key
}

//
// act[lookupKey] := valamilyen_cipher.Encrypt(access_key)

// end of pseudo code like code

// func (al *DefaultAccessLogic) CreateAccessKey(reference string) {
// }

func (al *DefaultAccessLogic) getLookUpKey(publisher ecdsa.PublicKey, tag string) ([]byte, error) {
	zeroByteArray := []byte{0}
	// Generate lookup key using Diffie Hellman
	lookup_key, err := al.diffieHellman.SharedSecret(&publisher, tag, zeroByteArray)
	if err != nil {
		return []byte{}, err
	}
	return lookup_key, nil

}

func (al *DefaultAccessLogic) getAccessKeyDecriptionKey(publisher ecdsa.PublicKey, tag string) ([]byte, error) {
	oneByteArray := []byte{1}
	// Generate access key decryption key using Diffie Hellman
	access_key_decryption_key, err := al.diffieHellman.SharedSecret(&publisher, tag, oneByteArray)
	if err != nil {
		return []byte{}, err
	}
	return access_key_decryption_key, nil
}

func (al *DefaultAccessLogic) getEncryptedAccessKey(act Act, lookup_key []byte) ([]byte, error) {
	val, err := act.Lookup(lookup_key)
	if err != nil {
		return []byte{}, err
	}
	return val, nil
}

func (al *DefaultAccessLogic) Get(act Act, encryped_ref swarm.Address, publisher ecdsa.PublicKey, tag string) (swarm.Address, error) {

	lookup_key, err := al.getLookUpKey(publisher, tag)
	if err != nil {
		return swarm.EmptyAddress, err
	}
	access_key_decryption_key, err := al.getAccessKeyDecriptionKey(publisher, tag)
	if err != nil {
		return swarm.EmptyAddress, err
	}

	// Lookup encrypted access key from the ACT manifest

	encrypted_access_key, err := al.getEncryptedAccessKey(act, lookup_key)
	if err != nil {
		return swarm.EmptyAddress, err
	}

	// Decrypt access key
	access_key_cipher := encryption.New(encryption.Key(access_key_decryption_key), 0, uint32(0), hashFunc)
	access_key, err := access_key_cipher.Decrypt(encrypted_access_key)
	if err != nil {
		return swarm.EmptyAddress, err
	}

	// Decrypt reference
	ref_cipher := encryption.New(access_key, 0, uint32(0), hashFunc)
	ref, err := ref_cipher.Decrypt(encryped_ref.Bytes())
	if err != nil {
		return swarm.EmptyAddress, err
	}

	return swarm.NewAddress(ref), nil
}

func NewAccessLogic(diffieHellman DiffieHellman) AccessLogic {
	return &DefaultAccessLogic{
		diffieHellman: diffieHellman,
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
