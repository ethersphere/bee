// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"errors"

	"github.com/ethersphere/bee/pkg/encryption"
)

var _ encryption.Interface = (*Encryptor)(nil)

var (
	// ErrNotImplemented is returned when a required Encryptor function is not set.
	ErrNotImplemented = errors.New("not implemented")
	// ErrInvalidXORKey is returned when the key for XOR encryption is not valid.
	ErrInvalidXORKey = errors.New("invalid xor key")
)

// Encryptor implements encryption Interface in order to mock it in tests.
type Encryptor struct {
	encryptFunc func(data []byte) ([]byte, error)
	decryptFunc func(data []byte) ([]byte, error)
	resetFunc   func()
	keyFunc     func() encryption.Key
}

// New returns a new Encryptor configured with provided options.
func New(opts ...Option) *Encryptor {
	e := new(Encryptor)
	for _, o := range opts {
		o(e)
	}
	return e
}

// Key has only bogus
func (e *Encryptor) Key() encryption.Key {
	if e.keyFunc == nil {
		return nil
	}
	return e.keyFunc()
}

// Encrypt calls the configured encrypt function, or returns ErrNotImplemented
// if it is not set.
func (e *Encryptor) Encrypt(data []byte) ([]byte, error) {
	if e.encryptFunc == nil {
		return nil, ErrNotImplemented
	}
	return e.encryptFunc(data)
}

// Decrypt calls the configured decrypt function, or returns ErrNotImplemented
// if it is not set.
func (e *Encryptor) Decrypt(data []byte) ([]byte, error) {
	if e.decryptFunc == nil {
		return nil, ErrNotImplemented
	}
	return e.decryptFunc(data)
}

// Reset calls the configured reset function, if it is set.
func (e *Encryptor) Reset() {
	if e.resetFunc == nil {
		return
	}
	e.resetFunc()
}

// Option represents configures the Encryptor instance.
type Option func(*Encryptor)

// WithEncryptFunc sets the Encryptor Encrypt function.
func WithEncryptFunc(f func([]byte) ([]byte, error)) Option {
	return func(e *Encryptor) {
		e.encryptFunc = f
	}
}

// WithDecryptFunc sets the Encryptor Decrypt function.
func WithDecryptFunc(f func([]byte) ([]byte, error)) Option {
	return func(e *Encryptor) {
		e.decryptFunc = f
	}
}

// WithResetFunc sets the Encryptor Reset function.
func WithResetFunc(f func()) Option {
	return func(e *Encryptor) {
		e.resetFunc = f
	}
}

// WithKeyFunc sets the Encryptor Key function.
func WithKeyFunc(f func() encryption.Key) Option {
	return func(e *Encryptor) {
		e.keyFunc = f
	}
}

// WithXOREncryption sets Encryptor Encrypt and Decrypt functions with XOR
// encryption function that uses the provided key for encryption.
func WithXOREncryption(key []byte) Option {
	f := newXORFunc(key)
	return func(e *Encryptor) {
		e.encryptFunc = f
		e.decryptFunc = f
	}
}

func newXORFunc(key []byte) func([]byte) ([]byte, error) {
	return func(data []byte) ([]byte, error) {
		return xor(data, key)
	}
}

func xor(input, key []byte) ([]byte, error) {
	keyLen := len(key)
	if keyLen == 0 {
		return nil, ErrInvalidXORKey
	}
	inputLen := len(input)
	output := make([]byte, inputLen)
	for i := 0; i < inputLen; i++ {
		output[i] = input[i] ^ key[i%keyLen]
	}
	return output, nil
}
