// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accesscontrol_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/accesscontrol"
	kvsmock "github.com/ethersphere/bee/v2/pkg/accesscontrol/kvs/mock"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/stretchr/testify/assert"
)

func assertNoError(t *testing.T, msg string, err error) {
	t.Helper()
	if err != nil {
		assert.FailNowf(t, err.Error(), msg)
	}
}

func assertError(t *testing.T, msg string, err error) {
	t.Helper()
	if err == nil {
		assert.FailNowf(t, fmt.Sprintf("Expected %s error, got nil", msg), "")
	}
}

// Generates a new test environment with a fix private key.
func setupAccessLogic() accesscontrol.ActLogic {
	privateKey := getPrivKey(1)
	diffieHellman := accesscontrol.NewDefaultSession(privateKey)
	al := accesscontrol.NewLogic(diffieHellman)

	return al
}

func getPrivKey(keyNumber int) *ecdsa.PrivateKey {
	var keyHex string

	switch keyNumber {
	case 0:
		keyHex = "a786dd84b61485de12146fd9c4c02d87e8fd95f0542765cb7fc3d2e428c0bcfa"
	case 1:
		keyHex = "b786dd84b61485de12146fd9c4c02d87e8fd95f0542765cb7fc3d2e428c0bcfb"
	case 2:
		keyHex = "c786dd84b61485de12146fd9c4c02d87e8fd95f0542765cb7fc3d2e428c0bcfc"
	default:
		panic("Invalid key number")
	}

	data, err := hex.DecodeString(keyHex)
	if err != nil {
		panic(err)
	}

	privKey, err := crypto.DecodeSecp256k1PrivateKey(data)
	if err != nil {
		panic(err)
	}

	return privKey
}

func TestDecryptRef_Publisher(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	id1 := getPrivKey(1)
	s := kvsmock.New()
	al := setupAccessLogic()
	err := al.AddGrantee(ctx, s, &id1.PublicKey, &id1.PublicKey)
	assertNoError(t, "AddGrantee", err)

	byteRef, err := hex.DecodeString("39a5ea87b141fe44aa609c3327ecd896c0e2122897f5f4bbacf74db1033c5559")
	assertNoError(t, "DecodeString", err)
	expectedRef := swarm.NewAddress(byteRef)
	encryptedRef, err := al.EncryptRef(ctx, s, &id1.PublicKey, expectedRef)
	assertNoError(t, "al encryptref", err)

	t.Run("decrypt success", func(t *testing.T) {
		actualRef, err := al.DecryptRef(ctx, s, encryptedRef, &id1.PublicKey)
		assertNoError(t, "decrypt ref", err)

		if !expectedRef.Equal(actualRef) {
			assert.FailNowf(t, fmt.Sprintf("DecryptRef gave back wrong Swarm reference! Expedted: %v, actual: %v", expectedRef, actualRef), "")
		}
	})
	t.Run("decrypt with nil publisher", func(t *testing.T) {
		_, err = al.DecryptRef(ctx, s, encryptedRef, nil)
		assertError(t, "al decryptref", err)
	})
}

func TestDecryptRefWithGrantee_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	id0, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	assertNoError(t, "GenerateKey", err)
	diffieHellman := accesscontrol.NewDefaultSession(id0)
	al := accesscontrol.NewLogic(diffieHellman)

	s := kvsmock.New()
	err = al.AddGrantee(ctx, s, &id0.PublicKey, &id0.PublicKey)
	assertNoError(t, "AddGrantee publisher", err)

	id1, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	assertNoError(t, "GenerateKey", err)
	err = al.AddGrantee(ctx, s, &id0.PublicKey, &id1.PublicKey)
	assertNoError(t, "AddGrantee id1", err)

	byteRef, err := hex.DecodeString("39a5ea87b141fe44aa609c3327ecd896c0e2122897f5f4bbacf74db1033c5559")
	assertNoError(t, "DecodeString", err)

	expectedRef := swarm.NewAddress(byteRef)

	encryptedRef, err := al.EncryptRef(ctx, s, &id0.PublicKey, expectedRef)
	assertNoError(t, "al encryptref", err)

	diffieHellman2 := accesscontrol.NewDefaultSession(id1)
	granteeAccessLogic := accesscontrol.NewLogic(diffieHellman2)
	actualRef, err := granteeAccessLogic.DecryptRef(ctx, s, encryptedRef, &id0.PublicKey)
	assertNoError(t, "grantee al decryptref", err)

	if !expectedRef.Equal(actualRef) {
		assert.FailNowf(t, fmt.Sprintf("DecryptRef gave back wrong Swarm reference! Expedted: %v, actual: %v", expectedRef, actualRef), "")
	}
}

func TestAddPublisher(t *testing.T) {
	t.Parallel()
	id0 := getPrivKey(0)
	savedLookupKey := "b6ee086390c280eeb9824c331a4427596f0c8510d5564bc1b6168d0059a46e2b"
	s := kvsmock.New()
	ctx := context.Background()

	al := setupAccessLogic()
	err := al.AddGrantee(ctx, s, &id0.PublicKey, &id0.PublicKey)
	assertNoError(t, "AddGrantee", err)

	decodedSavedLookupKey, err := hex.DecodeString(savedLookupKey)
	assertNoError(t, "decode LookupKey", err)

	encryptedAccessKey, err := s.Get(ctx, decodedSavedLookupKey)
	assertNoError(t, "kvs Get accesskey", err)

	decodedEncryptedAccessKey := hex.EncodeToString(encryptedAccessKey)

	// A random value is returned, so it is only possible to check the length of the returned value
	// We know the lookup key because the generated private key is fixed
	if len(decodedEncryptedAccessKey) != 64 {
		assert.FailNowf(t, fmt.Sprintf("AddGrantee: expected encrypted access key length 64, got %d", len(decodedEncryptedAccessKey)), "")
	}
}

func TestAddNewGranteeToContent(t *testing.T) {
	t.Parallel()
	id0 := getPrivKey(0)
	id1 := getPrivKey(1)
	id2 := getPrivKey(2)
	ctx := context.Background()

	publisherLookupKey := "b6ee086390c280eeb9824c331a4427596f0c8510d5564bc1b6168d0059a46e2b"
	firstAddedGranteeLookupKey := "a13678e81f9d939b9401a3ad7e548d2ceb81c50f8c76424296e83a1ad79c0df0"
	secondAddedGranteeLookupKey := "d5e9a6499ca74f5b8b958a4b89b7338045b2baa9420e115443a8050e26986564"

	s := kvsmock.New()
	al := setupAccessLogic()
	err := al.AddGrantee(ctx, s, &id0.PublicKey, &id0.PublicKey)
	assertNoError(t, "AddGrantee id0", err)

	err = al.AddGrantee(ctx, s, &id0.PublicKey, &id1.PublicKey)
	assertNoError(t, "AddGrantee id1", err)

	err = al.AddGrantee(ctx, s, &id0.PublicKey, &id2.PublicKey)
	assertNoError(t, "AddGrantee id2", err)

	lookupKeyAsByte, err := hex.DecodeString(publisherLookupKey)
	assertNoError(t, "publisher lookupkey DecodeString", err)

	result, err := s.Get(ctx, lookupKeyAsByte)
	assertNoError(t, "1st kvs get", err)
	hexEncodedEncryptedAK := hex.EncodeToString(result)
	if len(hexEncodedEncryptedAK) != 64 {
		assert.FailNowf(t, fmt.Sprintf("AddNewGrantee: expected encrypted access key length 64, got %d", len(hexEncodedEncryptedAK)), "")
	}

	lookupKeyAsByte, err = hex.DecodeString(firstAddedGranteeLookupKey)
	assertNoError(t, "1st lookupkey DecodeString", err)

	result, err = s.Get(ctx, lookupKeyAsByte)
	assertNoError(t, "2nd kvs get", err)
	hexEncodedEncryptedAK = hex.EncodeToString(result)
	if len(hexEncodedEncryptedAK) != 64 {
		assert.FailNowf(t, fmt.Sprintf("AddNewGrantee: expected encrypted access key length 64, got %d", len(hexEncodedEncryptedAK)), "")
	}

	lookupKeyAsByte, err = hex.DecodeString(secondAddedGranteeLookupKey)
	assertNoError(t, "2nd lookupkey DecodeString", err)

	result, err = s.Get(ctx, lookupKeyAsByte)
	assertNoError(t, "3rd kvs get", err)
	hexEncodedEncryptedAK = hex.EncodeToString(result)
	if len(hexEncodedEncryptedAK) != 64 {
		assert.FailNowf(t, fmt.Sprintf("AddNewGrantee: expected encrypted access key length 64, got %d", len(hexEncodedEncryptedAK)), "")
	}
}
