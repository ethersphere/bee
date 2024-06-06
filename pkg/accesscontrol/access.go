// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accesscontrol

import (
	"context"
	"crypto/ecdsa"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/accesscontrol/kvs"
	"github.com/ethersphere/bee/v2/pkg/encryption"
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
	// DecryptRef will return a decrypted reference, for given encrypted reference and grantee.
	DecryptRef(ctx context.Context, storage kvs.KeyValueStore, encryptedRef swarm.Address, publisher *ecdsa.PublicKey) (swarm.Address, error)
	Session
}

// Control interface for the ACT (does write operations).
type Control interface {
	Decryptor
	// AddGrantee adds a new grantee to the ACT.
	AddGrantee(ctx context.Context, storage kvs.KeyValueStore, publisherPubKey, granteePubKey *ecdsa.PublicKey) error
	// EncryptRef encrypts a Swarm reference for a given grantee.
	EncryptRef(ctx context.Context, storage kvs.KeyValueStore, grantee *ecdsa.PublicKey, ref swarm.Address) (swarm.Address, error)
}

// ActLogic represents the access control logic.
type ActLogic struct {
	Session
}

var _ Control = (*ActLogic)(nil)

// EncryptRef encrypts a Swarm reference for a publisher.
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
func (al ActLogic) AddGrantee(ctx context.Context, storage kvs.KeyValueStore, publisherPubKey, granteePubKey *ecdsa.PublicKey) error {
	var (
		accessKey encryption.Key
		err       error
	)

	// Create new access key because grantee is the publisher.
	if publisherPubKey.Equal(granteePubKey) {
		accessKey = encryption.GenerateRandomKey(encryption.KeyLength)
	} else {
		// Get previously generated access key.
		accessKey, err = al.getAccessKey(ctx, storage, publisherPubKey)
		if err != nil {
			return err
		}
	}

	lookupKey, accessKeyDecryptionKey, err := al.getKeys(granteePubKey)
	if err != nil {
		return err
	}

	// Encrypt the access key for the new Grantee.
	cipher := encryption.New(encryption.Key(accessKeyDecryptionKey), 0, uint32(0), hashFunc)
	granteeEncryptedAccessKey, err := cipher.Encrypt(accessKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt access key: %w", err)
	}

	// Add the new encrypted access key to the Act.
	err = storage.Put(ctx, lookupKey, granteeEncryptedAccessKey)
	if err != nil {
		return fmt.Errorf("failed to put value to KVS: %w", err)
	}

	return nil
}

// Will return the access key for a publisher (public key).
func (al *ActLogic) getAccessKey(ctx context.Context, storage kvs.KeyValueStore, publisherPubKey *ecdsa.PublicKey) ([]byte, error) {
	publisherLookupKey, publisherAKDecryptionKey, err := al.getKeys(publisherPubKey)
	if err != nil {
		return nil, err
	}
	// no need for constructor call if value not found in act.
	accessKeyDecryptionCipher := encryption.New(encryption.Key(publisherAKDecryptionKey), 0, uint32(0), hashFunc)
	encryptedAK, err := storage.Get(ctx, publisherLookupKey)
	if err != nil {
		return nil, fmt.Errorf("failed go get value from KVS: %w", err)
	}

	accessKey, err := accessKeyDecryptionCipher.Decrypt(encryptedAK)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt access key: %w", err)
	}

	return accessKey, nil
}

// Generate lookup key and access key decryption key for a given public key.
func (al *ActLogic) getKeys(publicKey *ecdsa.PublicKey) ([]byte, []byte, error) {
	nonces := [][]byte{zeroByteArray, oneByteArray}
	keys, err := al.Session.Key(publicKey, nonces)
	if len(keys) != len(nonces) {
		return nil, nil, err
	}
	return keys[0], keys[1], err
}

// DecryptRef will return a decrypted reference, for given encrypted reference and publisher.
func (al ActLogic) DecryptRef(ctx context.Context, storage kvs.KeyValueStore, encryptedRef swarm.Address, publisher *ecdsa.PublicKey) (swarm.Address, error) {
	accessKey, err := al.getAccessKey(ctx, storage, publisher)
	if err != nil {
		return swarm.ZeroAddress, err
	}

	refCipher := encryption.New(accessKey, 0, uint32(0), hashFunc)
	ref, err := refCipher.Decrypt(encryptedRef.Bytes())
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("failed to decrypt reference: %w", err)
	}

	return swarm.NewAddress(ref), nil
}

// NewLogic creates a new ACT Logic from a session.
func NewLogic(s Session) ActLogic {
	return ActLogic{
		Session: s,
	}
}
