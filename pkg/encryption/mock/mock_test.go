// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/ethersphere/bee/pkg/encryption/mock"
)

var errTest = errors.New("test error")

func TestEncryptor_Encrypt(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options []mock.Option
		data    []byte
		want    []byte
		wantErr error
	}{
		{
			name:    "empty",
			wantErr: mock.ErrNotImplemented,
		},
		{
			name: "func constant",
			options: []mock.Option{mock.WithEncryptFunc(func([]byte) ([]byte, error) {
				return []byte("some random data"), nil
			})},
			want: []byte("some random data"),
		},
		{
			name: "func identity",
			data: []byte("input data"),
			options: []mock.Option{mock.WithEncryptFunc(func(data []byte) ([]byte, error) {
				return data, nil
			})},
			want: []byte("input data"),
		},
		{
			name: "func err",
			options: []mock.Option{mock.WithEncryptFunc(func([]byte) ([]byte, error) {
				return nil, errTest
			})},
			wantErr: errTest,
		},
		{
			name:    "xor",
			data:    []byte("input data"),
			options: []mock.Option{mock.WithXOREncryption([]byte("the key"))},
			want:    []byte{0x1d, 0x6, 0x15, 0x55, 0x1f, 0x45, 0x1d, 0x15, 0x1c, 0x4},
		},
		{
			name:    "xor error",
			options: []mock.Option{mock.WithXOREncryption(nil)},
			wantErr: mock.ErrInvalidXORKey,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := mock.New(tc.options...).Encrypt(tc.data)
			if err != tc.wantErr {
				t.Fatalf("got error %v, want %v", err, tc.wantErr)
			}
			if !bytes.Equal(got, tc.want) {
				t.Errorf("got data %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestEncryptor_Decrypt(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options []mock.Option
		data    []byte
		want    []byte
		wantErr error
	}{
		{
			name:    "empty",
			wantErr: mock.ErrNotImplemented,
		},
		{
			name: "func constant",
			options: []mock.Option{mock.WithDecryptFunc(func([]byte) ([]byte, error) {
				return []byte("some random data"), nil
			})},
			want: []byte("some random data"),
		},
		{
			name: "func identity",
			data: []byte("input data"),
			options: []mock.Option{mock.WithDecryptFunc(func(data []byte) ([]byte, error) {
				return data, nil
			})},
			want: []byte("input data"),
		},
		{
			name: "func err",
			options: []mock.Option{mock.WithDecryptFunc(func([]byte) ([]byte, error) {
				return nil, errTest
			})},
			wantErr: errTest,
		},
		{
			name:    "xor",
			data:    []byte("input data"),
			options: []mock.Option{mock.WithXOREncryption([]byte("the key"))},
			want:    []byte{0x1d, 0x6, 0x15, 0x55, 0x1f, 0x45, 0x1d, 0x15, 0x1c, 0x4},
		},
		{
			name:    "xor error",
			options: []mock.Option{mock.WithXOREncryption(nil)},
			wantErr: mock.ErrInvalidXORKey,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := mock.New(tc.options...).Decrypt(tc.data)
			if err != tc.wantErr {
				t.Fatalf("got error %v, want %v", err, tc.wantErr)
			}
			if !bytes.Equal(got, tc.want) {
				t.Errorf("got data %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestEncryptor_Reset(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		// should not panic
		mock.New().Reset()
	})
	t.Run("func", func(t *testing.T) {
		var called bool
		mock.New(mock.WithResetFunc(func() {
			called = true
		})).Reset()
		if !called {
			t.Error("reset func not called")
		}
	})
}

func TestEncryptor_XOREncryption(t *testing.T) {
	key := []byte("some strong key")

	e := mock.New(mock.WithXOREncryption(key))

	data := []byte("very secret data")

	enc, err := e.Encrypt(data)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(enc, data) {
		t.Fatal("encrypted and input data must not be the same")
	}

	dec, err := e.Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(dec, data) {
		t.Errorf("got decrypted data %#v, want %#v", dec, data)
	}
}
