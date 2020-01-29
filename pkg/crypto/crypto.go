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

var keyTypeSecp256k1 = "Secp256k1"

func GenerateSecp256k1Key() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(btcec.S256(), rand.Reader)
}

type privateKey struct {
	Type string `json:"type"`
	Key  []byte `json:"key"`
}

func MarshalSecp256k1PrivateKey(k *ecdsa.PrivateKey) ([]byte, error) {
	return json.Marshal(privateKey{
		Type: keyTypeSecp256k1,
		Key:  (*btcec.PrivateKey)(k).Serialize(),
	})
}

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

func decodeSecp256k1PrivateKey(data []byte) (*ecdsa.PrivateKey, error) {
	if l := len(data); l != btcec.PrivKeyBytesLen {
		return nil, fmt.Errorf("secp256k1 data size %d expected %d", l, btcec.PrivKeyBytesLen)
	}
	privk, _ := btcec.PrivKeyFromBytes(btcec.S256(), data)
	return (*ecdsa.PrivateKey)(privk), nil
}

func NewAddress(p ecdsa.PublicKey) swarm.Address {
	d := elliptic.Marshal(btcec.S256(), p.X, p.Y)
	return swarm.NewAddress(keccak256(d))
}

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
