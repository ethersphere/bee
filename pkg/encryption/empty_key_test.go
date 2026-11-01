// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package encryption_test

import (
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/sha3"

	"github.com/ethersphere/bee/v2/pkg/encryption"
)

// TestEmptyKeyIsRejected guards against transform looping forever: it advances
// by the key length, so an empty key never made progress.
func TestEmptyKeyIsRejected(t *testing.T) {
	t.Parallel()

	for name, op := range map[string]func(encryption.Interface) error{
		"encrypt": func(e encryption.Interface) error { _, err := e.Encrypt([]byte("data")); return err },
		"decrypt": func(e encryption.Interface) error { _, err := e.Decrypt([]byte("data")); return err },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			done := make(chan error, 1)
			go func() { done <- op(encryption.New(nil, 0, 0, sha3.NewLegacyKeccak256)) }()

			select {
			case err := <-done:
				if !errors.Is(err, encryption.ErrInvalidKey) {
					t.Fatalf("got error %v, want %v", err, encryption.ErrInvalidKey)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("did not return: empty key reached the transform loop")
			}
		})
	}
}
