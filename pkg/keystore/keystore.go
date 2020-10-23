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
	// Key returns private key for specified name that was encrypted with
	// provided password. If the private key does not exists it creates new one
	// with name and password, and returns with created set to true.
	Key(name, password string) (k *ecdsa.PrivateKey, created bool, err error)
	// Exists returns true if the key with specified name exists.
	Exists(name string) (bool, error)
}
