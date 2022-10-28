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

	"github.com/ethersphere/bee/pkg/swarm"
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
func GenerateSecp256r1Key() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

// EncodeSecp256k1PrivateKey encodes raw ECDSA private key.
func EncodeSecp256r1PrivateKey(k *ecdsa.PrivateKey) ([]byte, error) {
	return x509.MarshalECPrivateKey(k)
}

// EncodeSecp256r1PublicKey encodes raw ECDSA public key in a 33-byte compressed format.
func EncodeSecp256r1PublicKey(k *ecdsa.PublicKey) []byte {
	panic("fix me")
}

// DecodeSecp256k1PrivateKey decodes raw ECDSA private key.
func DecodeSecp256r1PrivateKey(data []byte) (*ecdsa.PrivateKey, error) {
	return x509.ParseECPrivateKey(data)
}

// Secp256k1PrivateKeyFromBytes returns an ECDSA private key based on
// the byte slice.
func Secp256k1PrivateKeyFromBytes(data []byte) *ecdsa.PrivateKey {
	pk, _ := x509.ParseECPrivateKey(data)
	return pk
}

// NewEthereumAddress returns a binary representation of ethereum blockchain address.
// This function is based on github.com/ethereum/go-ethereum/crypto.PubkeyToAddress.
func NewEthereumAddress(p ecdsa.PublicKey) ([]byte, error) {
	if p.X == nil || p.Y == nil {
		return nil, errors.New("invalid public key")
	}
	pubBytes := elliptic.Marshal(elliptic.P256(), p.X, p.Y)
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
