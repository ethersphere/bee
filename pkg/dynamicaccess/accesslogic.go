package dynamicaccess

import (
	"context"
	"crypto/ecdsa"

	encryption "github.com/ethersphere/bee/v2/pkg/encryption"
	"github.com/ethersphere/bee/v2/pkg/kvs"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"golang.org/x/crypto/sha3"
)

var hashFunc = sha3.NewLegacyKeccak256

// Read-only interface for the ACT
type Decryptor interface {
	// DecryptRef will return a decrypted reference, for given encrypted reference and grantee
	DecryptRef(ctx context.Context, storage kvs.KeyValueStore, encryptedRef swarm.Address, publisher *ecdsa.PublicKey) (swarm.Address, error)
	// Embedding the Session interface
	Session
}

// Control interface for the ACT (does write operations)
type Control interface {
	// Embedding the Decryptor interface
	Decryptor
	// Adds a new grantee to the ACT
	AddGrantee(ctx context.Context, storage kvs.KeyValueStore, publisherPubKey, granteePubKey *ecdsa.PublicKey, accessKey *encryption.Key) error
	// Encrypts a Swarm reference for a given grantee
	EncryptRef(ctx context.Context, storage kvs.KeyValueStore, grantee *ecdsa.PublicKey, ref swarm.Address) (swarm.Address, error)
}

type ActLogic struct {
	Session
}

var _ Control = (*ActLogic)(nil)

// Adds a new publisher to an empty act
func (al ActLogic) AddPublisher(ctx context.Context, storage kvs.KeyValueStore, publisher *ecdsa.PublicKey) error {
	accessKey := encryption.GenerateRandomKey(encryption.KeyLength)

	return al.AddGrantee(ctx, storage, publisher, publisher, &accessKey)
}

// Encrypts a SWARM reference for a publisher
func (al ActLogic) EncryptRef(ctx context.Context, storage kvs.KeyValueStore, publisherPubKey *ecdsa.PublicKey, ref swarm.Address) (swarm.Address, error) {
	accessKey, err := al.getAccessKey(ctx, storage, publisherPubKey)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	refCipher := encryption.New(accessKey, 0, uint32(0), hashFunc)
	encryptedRef, _ := refCipher.Encrypt(ref.Bytes())

	return swarm.NewAddress(encryptedRef), nil
}

// Adds a new grantee to the ACT
func (al ActLogic) AddGrantee(ctx context.Context, storage kvs.KeyValueStore, publisherPubKey, granteePubKey *ecdsa.PublicKey, accessKeyPointer *encryption.Key) error {
	var accessKey encryption.Key
	var err error // Declare the "err" variable

	if accessKeyPointer == nil {
		// Get previously generated access key
		accessKey, err = al.getAccessKey(ctx, storage, publisherPubKey)
		if err != nil {
			return err
		}
	} else {
		// This is a newly created access key, because grantee is publisher (they are the same)
		accessKey = *accessKeyPointer
	}

	// Encrypt the access key for the new Grantee
	keys, err := al.getKeys(granteePubKey)
	if err != nil {
		return err
	}
	lookupKey := keys[0]
	// accessKeyDecryptionKey is used for encryption of the access key
	accessKeyDecryptionKey := keys[1]

	// Encrypt the access key for the new Grantee
	cipher := encryption.New(encryption.Key(accessKeyDecryptionKey), 0, uint32(0), hashFunc)
	granteeEncryptedAccessKey, err := cipher.Encrypt(accessKey)
	if err != nil {
		return err
	}

	// Add the new encrypted access key for the Act
	return storage.Put(ctx, lookupKey, granteeEncryptedAccessKey)
}

// Will return the access key for a publisher (public key)
func (al *ActLogic) getAccessKey(ctx context.Context, storage kvs.KeyValueStore, publisherPubKey *ecdsa.PublicKey) ([]byte, error) {
	keys, err := al.getKeys(publisherPubKey)
	if err != nil {
		return nil, err
	}
	publisherLookupKey := keys[0]
	publisherAKDecryptionKey := keys[1]
	// no need to constructor call if value not found in act
	accessKeyDecryptionCipher := encryption.New(encryption.Key(publisherAKDecryptionKey), 0, uint32(0), hashFunc)
	encryptedAK, err := storage.Get(ctx, publisherLookupKey)
	if err != nil {
		return nil, err
	}

	return accessKeyDecryptionCipher.Decrypt(encryptedAK)
}

var oneByteArray = []byte{1}
var zeroByteArray = []byte{0}

// Generate lookup key and access key decryption key for a given public key
func (al *ActLogic) getKeys(publicKey *ecdsa.PublicKey) ([][]byte, error) {
	return al.Session.Key(publicKey, [][]byte{zeroByteArray, oneByteArray})
}

// DecryptRef will return a decrypted reference, for given encrypted reference and publisher
func (al ActLogic) DecryptRef(ctx context.Context, storage kvs.KeyValueStore, encryptedRef swarm.Address, publisher *ecdsa.PublicKey) (swarm.Address, error) {
	keys, err := al.getKeys(publisher)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	lookupKey := keys[0]
	accessKeyDecryptionKey := keys[1]

	// Lookup encrypted access key from the ACT manifest
	encryptedAccessKey, err := storage.Get(ctx, lookupKey)
	if err != nil {
		return swarm.ZeroAddress, err
	}

	// Decrypt access key
	accessKeyCipher := encryption.New(encryption.Key(accessKeyDecryptionKey), 0, uint32(0), hashFunc)
	accessKey, err := accessKeyCipher.Decrypt(encryptedAccessKey)
	if err != nil {
		return swarm.ZeroAddress, err
	}

	// Decrypt reference
	refCipher := encryption.New(accessKey, 0, uint32(0), hashFunc)
	ref, err := refCipher.Decrypt(encryptedRef.Bytes())
	if err != nil {
		return swarm.ZeroAddress, err
	}

	return swarm.NewAddress(ref), nil
}

func NewLogic(s Session) ActLogic {
	return ActLogic{
		Session: s,
	}
}
