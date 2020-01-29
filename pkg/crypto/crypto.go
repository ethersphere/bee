// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"

	"github.com/btcsuite/btcd/btcec"
	"github.com/ethersphere/bee/pkg/swarm"
	"golang.org/x/crypto/sha3"
)

var keyTypeSecp256k1 = "secp256k1"

// GenerateSecp256k1Key generates an ECDSA private key using
// secp256k1 elliptic curve.
func GenerateSecp256k1Key() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(btcec.S256(), rand.Reader)
}

// NewAddress constructs a Swarm Address from ECDSA private key.
func NewAddress(p ecdsa.PublicKey) swarm.Address {
	d := elliptic.Marshal(btcec.S256(), p.X, p.Y)
	return swarm.NewAddress(keccak256(d))
}

// privateKey holds information about Swarm private key for mashaling.
type privateKey struct {
	Type string `json:"type"`
	Key  []byte `json:"key"`
}

// MarshalSecp256k1PrivateKey marshals secp256k1 ECDSA private key
// that can be unmarshaled by UnmarshalPrivateKey.
func MarshalSecp256k1PrivateKey(k *ecdsa.PrivateKey) ([]byte, error) {
	return json.Marshal(privateKey{
		Type: keyTypeSecp256k1,
		Key:  (*btcec.PrivateKey)(k).Serialize(),
	})
}

// UnmarshalPrivateKey unmarshals ECDSA private key from encoded data.
func UnmarshalPrivateKey(data []byte) (*ecdsa.PrivateKey, error) {
	var pk privateKey
	if err := json.Unmarshal(data, &pk); err != nil {
		return nil, err
	}
	switch t := pk.Type; t {
	case keyTypeSecp256k1:
		return decodeSecp256k1PrivateKey(pk.Key)
	default:
		return nil, fmt.Errorf("unknown key type %q", t)
	}
}

// decodeSecp256k1PrivateKey decodes raw ECDSA private key.
func decodeSecp256k1PrivateKey(data []byte) (*ecdsa.PrivateKey, error) {
	if l := len(data); l != btcec.PrivKeyBytesLen {
		return nil, fmt.Errorf("secp256k1 data size %d expected %d", l, btcec.PrivKeyBytesLen)
	}
	privk, _ := btcec.PrivKeyFromBytes(btcec.S256(), data)
	return (*ecdsa.PrivateKey)(privk), nil
}

// keccak256 calculates a hash sum from provided data.
func keccak256(data ...[]byte) []byte {
	d := sha3.New256()
	for _, b := range data {
		_, err := d.Write(b)
		if err != nil {
			panic(err)
		}
	}
	return d.Sum(nil)
}
