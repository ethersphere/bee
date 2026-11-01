// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha3"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/keystore"
	"github.com/google/uuid"
	"golang.org/x/crypto/scrypt"
)

var _ keystore.Service = (*Service)(nil)

const (
	keyHeaderKDF = "scrypt"
	keyVersion   = 3

	scryptN     = 1 << 15
	scryptR     = 8
	scryptP     = 1
	scryptDKLen = 32

	// maxScryptMem bounds the memory scrypt.Key is allowed to allocate for a
	// keyfile supplied set of parameters (it allocates 128*N*r bytes), so that
	// a malformed or hostile keyfile cannot exhaust the node's memory.
	maxScryptMem = 1 << 30
	// maxScryptP bounds the parallelization factor, which drives both the
	// number of sequential smix passes and the size of the pbkdf2 block.
	maxScryptP = 1 << 8
	// maxScryptDKLen bounds the derived key length, which is allocated verbatim
	// by pbkdf2. Anything shorter than scryptDKLen cannot satisfy the 32 bytes
	// this package reads out of the derived key.
	maxScryptDKLen = 1 << 16
)

// This format is compatible with Ethereum JSON v3 key file format.
type encryptedKey struct {
	Address string    `json:"address"`
	Crypto  keyCripto `json:"crypto"`
	Version int       `json:"version"`
	Id      string    `json:"id"`
}

type keyCripto struct {
	Cipher       string       `json:"cipher"`
	CipherText   string       `json:"ciphertext"`
	CipherParams cipherParams `json:"cipherparams"`
	KDF          string       `json:"kdf"`
	KDFParams    kdfParams    `json:"kdfparams"`
	MAC          string       `json:"mac"`
}

type cipherParams struct {
	IV string `json:"iv"`
}

type kdfParams struct {
	N     int    `json:"n"`
	R     int    `json:"r"`
	P     int    `json:"p"`
	DKLen int    `json:"dklen"`
	Salt  string `json:"salt"`
}

// validate checks that the scrypt parameters decoded from a keyfile are usable.
// Parameters are attacker controlled, so they are rejected rather than passed to
// scrypt.Key, which panics on a zero key length and happily allocates 128*N*r
// bytes for any well formed but oversized N and r.
func (p kdfParams) validate() error {
	switch {
	case p.DKLen < scryptDKLen || p.DKLen > maxScryptDKLen:
		return fmt.Errorf("invalid scrypt kdf parameters: dklen must be between %d and %d, got %d", scryptDKLen, maxScryptDKLen, p.DKLen)
	case p.N <= 1 || p.N&(p.N-1) != 0 || p.N > maxScryptMem/128:
		return fmt.Errorf("invalid scrypt kdf parameters: n must be a power of two between 2 and %d, got %d", maxScryptMem/128, p.N)
	case p.R <= 0 || p.R > maxScryptMem/128:
		return fmt.Errorf("invalid scrypt kdf parameters: r must be greater than 0 and at most %d, got %d", maxScryptMem/128, p.R)
	case p.P <= 0 || p.P > maxScryptP:
		return fmt.Errorf("invalid scrypt kdf parameters: p must be greater than 0 and at most %d, got %d", maxScryptP, p.P)
	// p.R is > 0 above, so this states N*128*R > maxScryptMem without the
	// multiplication overflowing or needing an unsigned conversion.
	case p.N > maxScryptMem/128/p.R:
		return fmt.Errorf("invalid scrypt kdf parameters: n and r require more than %d bytes of memory", maxScryptMem)
	}
	return nil
}

func encryptKey(k *ecdsa.PrivateKey, password string, edg keystore.EDG) ([]byte, error) {
	data, err := edg.Encode(k)
	if err != nil {
		return nil, err
	}
	kc, err := encryptData(data, []byte(password))
	if err != nil {
		return nil, err
	}
	var addr []byte
	switch k.Curve {
	case btcec.S256():
		a, err := crypto.NewEthereumAddress(k.PublicKey)
		if err != nil {
			return nil, err
		}
		addr = a
	case elliptic.P256():
		privKey, err := k.ECDH()
		if err != nil {
			return nil, fmt.Errorf("generate key: %w", err)
		}
		addr = privKey.PublicKey().Bytes()
	default:
		return nil, fmt.Errorf("unsupported curve: %v", k.Curve)
	}
	return json.Marshal(encryptedKey{
		Address: hex.EncodeToString(addr),
		Crypto:  *kc,
		Version: keyVersion,
		Id:      uuid.NewString(),
	})
}

func decryptKey(data []byte, password string, edg keystore.EDG) (*ecdsa.PrivateKey, error) {
	var k encryptedKey
	if err := json.Unmarshal(data, &k); err != nil {
		return nil, err
	}
	if k.Version != keyVersion {
		return nil, fmt.Errorf("unsupported key version: %v", k.Version)
	}
	d, err := decryptData(k.Crypto, password)
	if err != nil {
		return nil, err
	}
	return edg.Decode(d)
}

func encryptData(data, password []byte) (*keyCripto, error) {
	salt := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("read random data: %w", err)
	}
	derivedKey, err := scrypt.Key(password, salt, scryptN, scryptR, scryptP, scryptDKLen)
	if err != nil {
		return nil, err
	}
	encryptKey := derivedKey[:16]

	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, fmt.Errorf("read random data: %w", err)
	}
	cipherText, err := aesCTRXOR(encryptKey, data, iv)
	if err != nil {
		return nil, err
	}
	mac, err := crypto.LegacyKeccak256(append(derivedKey[16:32], cipherText...))
	if err != nil {
		return nil, err
	}

	return &keyCripto{
		Cipher:     "aes-128-ctr",
		CipherText: hex.EncodeToString(cipherText),
		CipherParams: cipherParams{
			IV: hex.EncodeToString(iv),
		},
		KDF: keyHeaderKDF,
		KDFParams: kdfParams{
			N:     scryptN,
			R:     scryptR,
			P:     scryptP,
			DKLen: scryptDKLen,
			Salt:  hex.EncodeToString(salt),
		},
		MAC: hex.EncodeToString(mac[:]),
	}, nil
}

func decryptData(v keyCripto, password string) ([]byte, error) {
	if v.Cipher != "aes-128-ctr" {
		return nil, fmt.Errorf("unsupported cipher: %v", v.Cipher)
	}

	mac, err := hex.DecodeString(v.MAC)
	if err != nil {
		return nil, fmt.Errorf("hex decode mac: %w", err)
	}
	cipherText, err := hex.DecodeString(v.CipherText)
	if err != nil {
		return nil, fmt.Errorf("hex decode cipher text: %w", err)
	}
	derivedKey, err := getKDFKey(v, []byte(password))
	if err != nil {
		return nil, err
	}
	calculatedMAC := sha3.Sum256(append(derivedKey[16:32], cipherText...))
	if !bytes.Equal(calculatedMAC[:], mac) {
		// if this fails we might be trying to load an ethereum V3 keyfile
		calculatedMACEth, err := crypto.LegacyKeccak256(append(derivedKey[16:32], cipherText...))
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(calculatedMACEth[:], mac) {
			return nil, keystore.ErrInvalidPassword
		}
	}

	iv, err := hex.DecodeString(v.CipherParams.IV)
	if err != nil {
		return nil, fmt.Errorf("hex decode IV cipher parameter: %w", err)
	}
	data, err := aesCTRXOR(derivedKey[:16], cipherText, iv)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func aesCTRXOR(key, inText, iv []byte) ([]byte, error) {
	aesBlock, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	stream := cipher.NewCTR(aesBlock, iv)
	outText := make([]byte, len(inText))
	stream.XORKeyStream(outText, inText)
	return outText, nil
}

func getKDFKey(v keyCripto, password []byte) ([]byte, error) {
	if v.KDF != keyHeaderKDF {
		return nil, fmt.Errorf("unsupported KDF: %s", v.KDF)
	}
	salt, err := hex.DecodeString(v.KDFParams.Salt)
	if err != nil {
		return nil, fmt.Errorf("hex decode salt: %w", err)
	}
	if err := v.KDFParams.validate(); err != nil {
		return nil, err
	}
	return scrypt.Key(
		password,
		salt,
		v.KDFParams.N,
		v.KDFParams.R,
		v.KDFParams.P,
		v.KDFParams.DKLen,
	)
}
