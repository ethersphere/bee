// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto

import (
	"crypto/ecdsa"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec"
	"github.com/ethereum/go-ethereum/core/types"
)

var (
	ErrInvalidLength = errors.New("invalid signature length")
)

type Signer interface {
	Sign(data []byte) ([]byte, error)
	SignTx(transaction *types.Transaction) (*types.Transaction, error)
	PublicKey() (*ecdsa.PublicKey, error)
	EthereumAddress() (EthAddress, error)
}

// addEthereumPrefix adds the ethereum prefix to the data
func addEthereumPrefix(data []byte) []byte {
	return []byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(data), data))
}

// hashWithEthereumPrefix returns the hash that should be signed for the given data
func hashWithEthereumPrefix(data []byte) ([]byte, error) {
	return LegacyKeccak256(addEthereumPrefix(data))
}

// Recover verifies signature with the data base provided.
// It is using `btcec.RecoverCompact` function
func Recover(signature, data []byte) (*ecdsa.PublicKey, error) {
	if len(signature) != 65 {
		return nil, ErrInvalidLength
	}
	// Convert to btcec input format with 'recovery id' v at the beginning.
	btcsig := make([]byte, 65)
	btcsig[0] = signature[64]
	copy(btcsig[1:], signature)

	hash, err := hashWithEthereumPrefix(data)
	if err != nil {
		return nil, err
	}

	p, _, err := btcec.RecoverCompact(btcec.S256(), btcsig, hash)
	return (*ecdsa.PublicKey)(p), err
}

type defaultSigner struct {
	key *ecdsa.PrivateKey
}

func NewDefaultSigner(key *ecdsa.PrivateKey) Signer {
	return &defaultSigner{
		key: key,
	}
}

func (d *defaultSigner) PublicKey() (*ecdsa.PublicKey, error) {
	return &d.key.PublicKey, nil
}

func (d *defaultSigner) Sign(data []byte) (signature []byte, err error) {
	hash, err := hashWithEthereumPrefix(data)
	if err != nil {
		return nil, err
	}

	signature, err = btcec.SignCompact(btcec.S256(), (*btcec.PrivateKey)(d.key), hash, true)
	if err != nil {
		return nil, err
	}

	// Convert to Ethereum signature format with 'recovery id' v at the end.
	v := signature[0]
	copy(signature, signature[1:])
	signature[64] = v
	return signature, nil
}

func (d *defaultSigner) SignTx(transaction *types.Transaction) (*types.Transaction, error) {
	hash := (&types.HomesteadSigner{}).Hash(transaction).Bytes()
	// isCompressedKey is false here so we get the expected v value (27 or 28)
	signature, err := btcec.SignCompact(btcec.S256(), (*btcec.PrivateKey)(d.key), hash, false)
	if err != nil {
		return nil, err
	}

	// Convert to Ethereum signature format with 'recovery id' v at the end.
	v := signature[0]
	copy(signature, signature[1:])
	// v value needs to be adjusted by 27 as transaction.WithSignature expects it to be 0 or 1
	signature[64] = v - 27
	return transaction.WithSignature(&types.HomesteadSigner{}, signature)
}

type EthAddress = [20]byte

func (d *defaultSigner) EthereumAddress() (EthAddress, error) {
	publicKey, err := d.PublicKey()
	if err != nil {
		return EthAddress{}, err
	}
	eth, err := NewEthereumAddress(*publicKey)
	if err != nil {
		return EthAddress{}, err
	}
	var ethAddress EthAddress
	copy(ethAddress[:], eth)
	return ethAddress, nil
}
