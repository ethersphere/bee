// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/binary"
	"fmt"

	"github.com/btcsuite/btcd/btcec"
	"github.com/ethersphere/bee/pkg/swarm"
	"golang.org/x/crypto/sha3"
)

// NewOverlayAddress constructs a Swarm Address from ECDSA private key.
func NewOverlayAddress(p ecdsa.PublicKey, networkID uint64) swarm.Address {
	ethAddr := NewEthereumAddress(p)
	netIDBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(netIDBytes, networkID)
	h := sha3.Sum256(append(ethAddr, netIDBytes...))
	return swarm.NewAddress(h[:])
}

// GenerateSecp256k1Key generates an ECDSA private key using
// secp256k1 elliptic curve.
func GenerateSecp256k1Key() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(btcec.S256(), rand.Reader)
}

// EncodeSecp256k1PrivateKey encodes raw ECDSA private key.
func EncodeSecp256k1PrivateKey(k *ecdsa.PrivateKey) []byte {
	return (*btcec.PrivateKey)(k).Serialize()
}

// DecodeSecp256k1PrivateKey decodes raw ECDSA private key.
func DecodeSecp256k1PrivateKey(data []byte) (*ecdsa.PrivateKey, error) {
	if l := len(data); l != btcec.PrivKeyBytesLen {
		return nil, fmt.Errorf("secp256k1 data size %d expected %d", l, btcec.PrivKeyBytesLen)
	}
	privk, _ := btcec.PrivKeyFromBytes(btcec.S256(), data)
	return (*ecdsa.PrivateKey)(privk), nil
}

// NewEthereumAddress returns a bunary representation of ethereum blockchain address.
// This function is based on github.com/ethereum/go-ethereum/crypto.PubkeyToAddress.
func NewEthereumAddress(p ecdsa.PublicKey) []byte {
	if p.X == nil || p.Y == nil {
		return nil
	}
	pubBytes := elliptic.Marshal(btcec.S256(), p.X, p.Y)
	return legacyKeccak256(pubBytes[1:])[12:]
}

func legacyKeccak256(data []byte) []byte {
	return sha3.NewLegacyKeccak256().Sum(data)
}
