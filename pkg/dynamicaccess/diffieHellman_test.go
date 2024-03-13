package dynamicaccess_test

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"io"
	"testing"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/dynamicaccess"
)

func TestSharedSecret(t *testing.T) {
	pk, _ := crypto.GenerateSecp256k1Key()
	_, err := dynamicaccess.NewDiffieHellman(pk).SharedSecret(&pk.PublicKey, "", nil)
	if err != nil {
		t.Errorf("Error generating shared secret: %v", err)
	}
}

func TestECDHCorrect(t *testing.T) {
	t.Parallel()

	key1, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	dh1 := dynamicaccess.NewDiffieHellman(key1)

	key2, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	dh2 := dynamicaccess.NewDiffieHellman(key2)

	moment := make([]byte, 1)
	if _, err := io.ReadFull(rand.Reader, moment); err != nil {
		t.Fatal(err)
	}

	shared1, err := dh1.SharedSecret(&key2.PublicKey, "", moment)
	if err != nil {
		t.Fatal(err)
	}
	shared2, err := dh2.SharedSecret(&key1.PublicKey, "", moment)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(shared1, shared2) {
		t.Fatalf("shared secrets do not match %s, %s", hex.EncodeToString(shared1), hex.EncodeToString(shared2))
	}
}
