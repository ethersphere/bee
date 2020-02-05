// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package keystore

import (
	"crypto/ecdsa"
	"errors"
)

var ErrInvalidPassword = errors.New("invalid password")

type Service interface {
	Key(name, password string) (k *ecdsa.PrivateKey, created bool, err error)
}
