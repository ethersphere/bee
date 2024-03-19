package dynamicaccess_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"io"
	"testing"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/dynamicaccess"
	"github.com/ethersphere/bee/pkg/dynamicaccess/mock"
	memkeystore "github.com/ethersphere/bee/pkg/keystore/mem"
)

func mockKeyFunc(publicKey *ecdsa.PublicKey, nonces [][]byte) ([][]byte, error) {
	return [][]byte{{1}}, nil
}

func TestSessionNewDefaultSession(t *testing.T) {
	pk, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatalf("Error generating private key: %v", err)
	}
	si := dynamicaccess.NewDefaultSession(pk)
	if si == nil {
		t.Fatal("Session instance is nil")
	}
}

func TestSessionNewFromKeystore(t *testing.T) {
	ks := memkeystore.New()
	si := mock.NewFromKeystore(ks, "tag", "password", mockKeyFunc)
	if si == nil {
		t.Fatal("Session instance is nil")
	}
}

func TestSessionKey(t *testing.T) {
	t.Parallel()

	key1, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	si1 := dynamicaccess.NewDefaultSession(key1)

	key2, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	si2 := dynamicaccess.NewDefaultSession(key2)

	nonces := make([][]byte, 1)
	if _, err := io.ReadFull(rand.Reader, nonces[0]); err != nil {
		t.Fatal(err)
	}

	keys1, err := si1.Key(&key2.PublicKey, nonces)
	if err != nil {
		t.Fatal(err)
	}
	keys2, err := si2.Key(&key1.PublicKey, nonces)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(keys1[0], keys2[0]) {
		t.Fatalf("shared secrets do not match %s, %s", hex.EncodeToString(keys1[0]), hex.EncodeToString(keys2[0]))
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
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("Key1 should exist")
	}
	key1, created, err := ks.Key(tag1, password1, crypto.EDGSecp256_K1)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("Key1 should not be created")
	}

	si2 := mock.NewFromKeystore(ks, tag2, password2, mockKeyFunc)
	exists, err = ks.Exists(tag2)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("Key2 should exist")
	}
	key2, created, err := ks.Key(tag2, password2, crypto.EDGSecp256_K1)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("Key2 should not be created")
	}

	nonces := make([][]byte, 1)
	if _, err := io.ReadFull(rand.Reader, nonces[0]); err != nil {
		t.Fatal(err)
	}

	keys1, err := si1.Key(&key2.PublicKey, nonces)
	if err != nil {
		t.Fatal(err)
	}
	keys2, err := si2.Key(&key1.PublicKey, nonces)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(keys1[0], keys2[0]) {
		t.Fatalf("shared secrets do not match %s, %s", hex.EncodeToString(keys1[0]), hex.EncodeToString(keys2[0]))
	}
}
