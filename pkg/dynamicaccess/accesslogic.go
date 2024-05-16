// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dynamicaccess

import (
	"context"
	"crypto/ecdsa"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/encryption"
	"github.com/ethersphere/bee/v2/pkg/kvs"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"golang.org/x/crypto/sha3"
)

//nolint:gochecknoglobals
var (
	hashFunc      = sha3.NewLegacyKeccak256
	oneByteArray  = []byte{1}
	zeroByteArray = []byte{0}
)

// Decryptor is a read-only interface for the ACT.
type Decryptor interface {
	// DecryptRef will return a decrypted reference, for given encrypted reference and grantee
	DecryptRef(ctx context.Context, storage kvs.KeyValueStore, encryptedRef swarm.Address, publisher *ecdsa.PublicKey) (swarm.Address, error)
	Session
}

// Control interface for the ACT (does write operations).
type Control interface {
	Decryptor
	// AddGrantee adds a new grantee to the ACT
	AddGrantee(ctx context.Context, storage kvs.KeyValueStore, publisherPubKey, granteePubKey *ecdsa.PublicKey, accessKey *encryption.Key) error
	// EncryptRef encrypts a Swarm reference for a given grantee
	EncryptRef(ctx context.Context, storage kvs.KeyValueStore, grantee *ecdsa.PublicKey, ref swarm.Address) (swarm.Address, error)
}

type ActLogic struct {
	Session
}

var _ Control = (*ActLogic)(nil)

// AddPublisher adds a new publisher to an empty act.
func (al ActLogic) AddPublisher(ctx context.Context, storage kvs.KeyValueStore, publisher *ecdsa.PublicKey) error {
	accessKey := encryption.GenerateRandomKey(encryption.KeyLength)

	return al.AddGrantee(ctx, storage, publisher, publisher, &accessKey)
}

// EncryptRef encrypts a SWARM reference for a publisher.
func (al ActLogic) EncryptRef(ctx context.Context, storage kvs.KeyValueStore, publisherPubKey *ecdsa.PublicKey, ref swarm.Address) (swarm.Address, error) {
	accessKey, err := al.getAccessKey(ctx, storage, publisherPubKey)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	refCipher := encryption.New(accessKey, 0, uint32(0), hashFunc)
	encryptedRef, err := refCipher.Encrypt(ref.Bytes())
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("failed to encrypt reference: %w", err)
	}

	return swarm.NewAddress(encryptedRef), nil
}

// AddGrantee adds a new grantee to the ACT.
func (al ActLogic) AddGrantee(ctx context.Context, storage kvs.KeyValueStore, publisherPubKey, granteePubKey *ecdsa.PublicKey, accessKeyPointer *encryption.Key) error {
	var (
		accessKey encryption.Key
		err       error
	)

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
	lookupKey, accessKeyDecryptionKey, err := al.getKeys(granteePubKey)
	if err != nil {
		return err
	}

	// Encrypt the access key for the new Grantee
	cipher := encryption.New(encryption.Key(accessKeyDecryptionKey), 0, uint32(0), hashFunc)
	granteeEncryptedAccessKey, err := cipher.Encrypt(accessKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt access key: %w", err)
	}

	// Add the new encrypted access key to the Act
	return storage.Put(ctx, lookupKey, granteeEncryptedAccessKey)
}

// Will return the access key for a publisher (public key).
func (al *ActLogic) getAccessKey(ctx context.Context, storage kvs.KeyValueStore, publisherPubKey *ecdsa.PublicKey) ([]byte, error) {
	publisherLookupKey, publisherAKDecryptionKey, err := al.getKeys(publisherPubKey)
	if err != nil {
		return nil, err
	}
	// no need for constructor call if value not found in act
	accessKeyDecryptionCipher := encryption.New(encryption.Key(publisherAKDecryptionKey), 0, uint32(0), hashFunc)
	encryptedAK, err := storage.Get(ctx, publisherLookupKey)
	if err != nil {
		return nil, fmt.Errorf("failed go get value from KVS: %w", err)
	}

	return accessKeyDecryptionCipher.Decrypt(encryptedAK)
}

// Generate lookup key and access key decryption key for a given public key
func (al *ActLogic) getKeys(publicKey *ecdsa.PublicKey) ([]byte, []byte, error) {
	nonces := [][]byte{zeroByteArray, oneByteArray}
	keys, err := al.Session.Key(publicKey, nonces)
	if keys == nil {
		return nil, nil, err
	}
	return keys[0], keys[1], err
}

// DecryptRef will return a decrypted reference, for given encrypted reference and publisher
func (al ActLogic) DecryptRef(ctx context.Context, storage kvs.KeyValueStore, encryptedRef swarm.Address, publisher *ecdsa.PublicKey) (swarm.Address, error) {
	lookupKey, accessKeyDecryptionKey, err := al.getKeys(publisher)
	if err != nil {
		return swarm.ZeroAddress, err
	}

	// Lookup encrypted access key from the ACT manifest
	encryptedAccessKey, err := storage.Get(ctx, lookupKey)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("failed to get access key from KVS: %w", err)
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
