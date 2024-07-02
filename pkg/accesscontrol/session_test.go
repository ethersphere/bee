// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accesscontrol_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/accesscontrol"
	"github.com/ethersphere/bee/v2/pkg/accesscontrol/mock"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	memkeystore "github.com/ethersphere/bee/v2/pkg/keystore/mem"
	"github.com/stretchr/testify/assert"
)

func mockKeyFunc(*ecdsa.PublicKey, [][]byte) ([][]byte, error) {
	return [][]byte{{1}}, nil
}

func TestSessionNewDefaultSession(t *testing.T) {
	pk, err := crypto.GenerateSecp256k1Key()
	assertNoError(t, "generating private key", err)
	si := accesscontrol.NewDefaultSession(pk)
	if si == nil {
		assert.FailNow(t, "Session instance is nil")
	}
}

func TestSessionNewFromKeystore(t *testing.T) {
	ks := memkeystore.New()
	si := mock.NewFromKeystore(ks, "tag", "password", mockKeyFunc)
	if si == nil {
		assert.FailNow(t, "Session instance is nil")
	}
}

func TestSessionKey(t *testing.T) {
	t.Parallel()

	key1, err := crypto.GenerateSecp256k1Key()
	assertNoError(t, "key1 GenerateSecp256k1Key", err)
	si1 := accesscontrol.NewDefaultSession(key1)

	key2, err := crypto.GenerateSecp256k1Key()
	assertNoError(t, "key2 GenerateSecp256k1Key", err)
	si2 := accesscontrol.NewDefaultSession(key2)

	nonces := make([][]byte, 2)
	for i := range nonces {
		if _, err := io.ReadFull(rand.Reader, nonces[i]); err != nil {
			assert.FailNow(t, err.Error())
		}
	}

	keys1, err := si1.Key(&key2.PublicKey, nonces)
	assertNoError(t, "", err)
	keys2, err := si2.Key(&key1.PublicKey, nonces)
	assertNoError(t, "", err)

	if !bytes.Equal(keys1[0], keys2[0]) {
		assert.FailNowf(t, fmt.Sprintf("shared secrets do not match %s, %s", hex.EncodeToString(keys1[0]), hex.EncodeToString(keys2[0])), "")
	}
	if !bytes.Equal(keys1[1], keys2[1]) {
		assert.FailNowf(t, fmt.Sprintf("shared secrets do not match %s, %s", hex.EncodeToString(keys1[1]), hex.EncodeToString(keys2[1])), "")
	}
}

func TestSessionKeyWithoutNonces(t *testing.T) {
	t.Parallel()

	key1, err := crypto.GenerateSecp256k1Key()
	assertNoError(t, "key1 GenerateSecp256k1Key", err)
	si1 := accesscontrol.NewDefaultSession(key1)

	key2, err := crypto.GenerateSecp256k1Key()
	assertNoError(t, "key2 GenerateSecp256k1Key", err)
	si2 := accesscontrol.NewDefaultSession(key2)

	keys1, err := si1.Key(&key2.PublicKey, nil)
	assertNoError(t, "session1 key", err)
	keys2, err := si2.Key(&key1.PublicKey, nil)
	assertNoError(t, "session2 key", err)

	if !bytes.Equal(keys1[0], keys2[0]) {
		assert.FailNowf(t, fmt.Sprintf("shared secrets do not match %s, %s", hex.EncodeToString(keys1[0]), hex.EncodeToString(keys2[0])), "")
	}
}

func TestSessionKeyFromKeystore(t *testing.T) {
	t.Parallel()

	ks := memkeystore.New()
	tag1 := "tag1"
	tag2 := "tag2"
	password1 := "password1"
	password2 := "password2"

	si1 := mock.NewFromKeystore(ks, tag1, password1, mockKeyFunc)
	exists, err := ks.Exists(tag1)
	assertNoError(t, "", err)
	if !exists {
		assert.FailNow(t, "Key1 should exist")

	}
	key1, created, err := ks.Key(tag1, password1, crypto.EDGSecp256_K1)
	assertNoError(t, "", err)
	if created {
		assert.FailNow(t, "Key1 should not be created")

	}

	si2 := mock.NewFromKeystore(ks, tag2, password2, mockKeyFunc)
	exists, err = ks.Exists(tag2)
	assertNoError(t, "", err)
	if !exists {
		assert.FailNow(t, "Key2 should exist")
	}
	key2, created, err := ks.Key(tag2, password2, crypto.EDGSecp256_K1)
	assertNoError(t, "", err)
	if created {
		assert.FailNow(t, "Key2 should not be created")
	}

	nonces := make([][]byte, 1)
	for i := range nonces {
		if _, err := io.ReadFull(rand.Reader, nonces[i]); err != nil {
			assert.FailNow(t, err.Error())
		}
	}

	keys1, err := si1.Key(&key2.PublicKey, nonces)
	assertNoError(t, "", err)
	keys2, err := si2.Key(&key1.PublicKey, nonces)
	assertNoError(t, "", err)

	if !bytes.Equal(keys1[0], keys2[0]) {
		assert.FailNowf(t, fmt.Sprintf("shared secrets do not match %s, %s", hex.EncodeToString(keys1[0]), hex.EncodeToString(keys2[0])), "")
	}
}
