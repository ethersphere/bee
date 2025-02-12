// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"golang.org/x/crypto/sha3"
)

// RecoverFunc is a function to recover the public key from a signature
type RecoverFunc func(signature, data []byte) (*ecdsa.PublicKey, error)

var ErrBadHashLength = errors.New("wrong block hash length")

const (
	AddressSize = 20
)

// NewOverlayAddress constructs a Swarm Address from ECDSA public key.
func NewOverlayAddress(p ecdsa.PublicKey, networkID uint64, nonce []byte) (swarm.Address, error) {
	ethAddr, err := NewEthereumAddress(p)
	if err != nil {
		return swarm.ZeroAddress, err
	}

	if len(nonce) != 32 {
		return swarm.ZeroAddress, ErrBadHashLength
	}

	return NewOverlayFromEthereumAddress(ethAddr, networkID, nonce)
}

// NewOverlayFromEthereumAddress constructs a Swarm Address for an Ethereum address.
func NewOverlayFromEthereumAddress(ethAddr []byte, networkID uint64, nonce []byte) (swarm.Address, error) {
	netIDBytes := make([]byte, 8)

	binary.LittleEndian.PutUint64(netIDBytes, networkID)

	data := append(ethAddr, netIDBytes...)
	data = append(data, nonce...)
	h, err := LegacyKeccak256(data)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	return swarm.NewAddress(h[:]), nil
}

// GenerateSecp256k1Key generates an ECDSA private key using
// secp256k1 elliptic curve.
func GenerateSecp256k1Key() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(btcec.S256(), rand.Reader)
}

// EncodeSecp256k1PrivateKey encodes raw ECDSA private key.
func EncodeSecp256k1PrivateKey(k *ecdsa.PrivateKey) ([]byte, error) {
	pvk, _ := btcec.PrivKeyFromBytes(k.D.Bytes())
	return pvk.Serialize(), nil
}

// EncodeSecp256k1PublicKey encodes raw ECDSA public key in a 33-byte compressed format.
func EncodeSecp256k1PublicKey(k *ecdsa.PublicKey) []byte {
	var x, y btcec.FieldVal
	x.SetByteSlice(k.X.Bytes())
	y.SetByteSlice(k.Y.Bytes())
	return btcec.NewPublicKey(&x, &y).SerializeCompressed()
}

// DecodeSecp256k1PrivateKey decodes raw ECDSA private key.
func DecodeSecp256k1PrivateKey(data []byte) (*ecdsa.PrivateKey, error) {
	if l := len(data); l != btcec.PrivKeyBytesLen {
		return nil, fmt.Errorf("secp256k1 data size %d expected %d", l, btcec.PrivKeyBytesLen)
	}
	pvk, _ := btcec.PrivKeyFromBytes(data)
	return pvk.ToECDSA(), nil
}

// GenerateSecp256k1Key generates an ECDSA private key using
// secp256k1 elliptic curve.
func GenerateSecp256r1Key() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

// EncodeSecp256k1PrivateKey encodes raw ECDSA private key.
func EncodeSecp256r1PrivateKey(k *ecdsa.PrivateKey) ([]byte, error) {
	return x509.MarshalECPrivateKey(k)
}

// DecodeSecp256k1PrivateKey decodes raw ECDSA private key.
func DecodeSecp256r1PrivateKey(data []byte) (*ecdsa.PrivateKey, error) {
	return x509.ParseECPrivateKey(data)
}

// Secp256k1PrivateKeyFromBytes returns an ECDSA private key based on
// the byte slice.
func Secp256k1PrivateKeyFromBytes(data []byte) *ecdsa.PrivateKey {
	pvk, _ := btcec.PrivKeyFromBytes(data)
	return pvk.ToECDSA()
}

// NewEthereumAddress returns a binary representation of ethereum blockchain address.
// This function is based on github.com/ethereum/go-ethereum/crypto.PubkeyToAddress.
func NewEthereumAddress(p ecdsa.PublicKey) ([]byte, error) {
	if p.X == nil || p.Y == nil {
		return nil, errors.New("invalid public key")
	}
	pubBytes := crypto.S256().Marshal(p.X, p.Y)
	pubHash, err := LegacyKeccak256(pubBytes[1:])
	if err != nil {
		return nil, err
	}
	return pubHash[12:], err
}

func LegacyKeccak256(data []byte) ([]byte, error) {
	var err error
	hasher := sha3.NewLegacyKeccak256()
	_, err = hasher.Write(data)
	if err != nil {
		return nil, err
	}
	return hasher.Sum(nil), err
}
