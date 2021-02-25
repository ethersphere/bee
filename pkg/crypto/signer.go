// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"

	"github.com/btcsuite/btcd/btcec"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/crypto/eip712"
)

var (
	ErrInvalidLength = errors.New("invalid signature length")
)

type Signer interface {
	// Sign signs data with ethereum prefix (eip191 type 0x45).
	Sign(data []byte) ([]byte, error)
	// SignTx signs an ethereum transaction.
	SignTx(transaction *types.Transaction, chainID *big.Int) (*types.Transaction, error)
	// SignTypedData signs data according to eip712.
	SignTypedData(typedData *eip712.TypedData) ([]byte, error)
	// PublicKey returns the public key this signer uses.
	PublicKey() (*ecdsa.PublicKey, error)
	// EthereumAddress returns the ethereum address this signer uses.
	EthereumAddress() (common.Address, error)
}

// addEthereumPrefix adds the ethereum prefix to the data.
func addEthereumPrefix(data []byte) []byte {
	return []byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(data), data))
}

// hashWithEthereumPrefix returns the hash that should be signed for the given data.
func hashWithEthereumPrefix(data []byte) ([]byte, error) {
	return LegacyKeccak256(addEthereumPrefix(data))
}

// Recover verifies signature with the data base provided.
// It is using `btcec.RecoverCompact` function.
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

// PublicKey returns the public key this signer uses.
func (d *defaultSigner) PublicKey() (*ecdsa.PublicKey, error) {
	return &d.key.PublicKey, nil
}

// Sign signs data with ethereum prefix (eip191 type 0x45).
func (d *defaultSigner) Sign(data []byte) (signature []byte, err error) {
	hash, err := hashWithEthereumPrefix(data)
	if err != nil {
		return nil, err
	}

	return d.sign(hash, true)
}

// SignTx signs an ethereum transaction.
func (d *defaultSigner) SignTx(transaction *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	txSigner := types.NewEIP155Signer(chainID)
	hash := txSigner.Hash(transaction).Bytes()
	// isCompressedKey is false here so we get the expected v value (27 or 28)
	signature, err := d.sign(hash, false)
	if err != nil {
		return nil, err
	}

	// v value needs to be adjusted by 27 as transaction.WithSignature expects it to be 0 or 1
	signature[64] -= 27
	return transaction.WithSignature(txSigner, signature)
}

// EthereumAddress returns the ethereum address this signer uses.
func (d *defaultSigner) EthereumAddress() (common.Address, error) {
	publicKey, err := d.PublicKey()
	if err != nil {
		return common.Address{}, err
	}
	eth, err := NewEthereumAddress(*publicKey)
	if err != nil {
		return common.Address{}, err
	}
	var ethAddress common.Address
	copy(ethAddress[:], eth)
	return ethAddress, nil
}

// SignTypedData signs data according to eip712.
func (d *defaultSigner) SignTypedData(typedData *eip712.TypedData) ([]byte, error) {
	rawData, err := eip712.EncodeForSigning(typedData)
	if err != nil {
		return nil, err
	}

	sighash, err := LegacyKeccak256(rawData)
	if err != nil {
		return nil, err
	}

	return d.sign(sighash, false)
}

// sign the provided hash and convert it to the ethereum (r,s,v) format.
func (d *defaultSigner) sign(sighash []byte, isCompressedKey bool) ([]byte, error) {
	signature, err := btcec.SignCompact(btcec.S256(), (*btcec.PrivateKey)(d.key), sighash, false)
	if err != nil {
		return nil, err
	}

	// Convert to Ethereum signature format with 'recovery id' v at the end.
	v := signature[0]
	copy(signature, signature[1:])
	signature[64] = v
	return signature, nil
}

// RecoverEIP712 recovers the public key for eip712 signed data.
func RecoverEIP712(signature []byte, data *eip712.TypedData) (*ecdsa.PublicKey, error) {
	if len(signature) != 65 {
		return nil, errors.New("invalid length")
	}
	// Convert to btcec input format with 'recovery id' v at the beginning.
	btcsig := make([]byte, 65)
	btcsig[0] = signature[64]
	copy(btcsig[1:], signature)

	rawData, err := eip712.EncodeForSigning(data)
	if err != nil {
		return nil, err
	}

	sighash, err := LegacyKeccak256(rawData)
	if err != nil {
		return nil, err
	}

	p, _, err := btcec.RecoverCompact(btcec.S256(), btcsig, sighash)
	return (*ecdsa.PublicKey)(p), err
}
