// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package keystore

import (
	"crypto/ecdsa"
	"errors"
)

// ErrInvalidPassword is returned when the password for decrypting content where
// private key is stored is not valid.
var ErrInvalidPassword = errors.New("invalid password")

// Service for managing keystore private keys.
type Service interface {
	// Key returns the private key for a specified name that was encrypted with
	// the provided password. If the private key does not exists it creates
	// a new one with a name and the password, and returns with created set
	// to true.
	Key(name, password string, generateFunc func() (*ecdsa.PrivateKey, error), encodeFunc func(k *ecdsa.PrivateKey) ([]byte, error), decodeFunc func(data []byte) (*ecdsa.PrivateKey, error)) (pk *ecdsa.PrivateKey, created bool, err error)
	// Exists returns true if the key with specified name exists.
	Exists(name string) (bool, error)
	// SetKey
	SetKey(name, password string, generateFunc func() (*ecdsa.PrivateKey, error), encodeFunc func(k *ecdsa.PrivateKey) ([]byte, error)) (*ecdsa.PrivateKey, error)
}
