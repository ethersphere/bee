// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package encryption_test

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/encryption"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"golang.org/x/crypto/sha3"
)

// FuzzDecrypt drives encryption.Encryption.Decrypt (and the underlying
// counter-mode Transcrypt transform) with fuzzed ciphertext under a fixed key.
// Decrypt reads back attacker-influenced bytes, so it must tolerate any input
// without panicking. Two invariants are asserted on the padding-free decrypter:
// a nil error implies the output length equals the input length, and decrypting
// a freshly produced ciphertext reproduces the original plaintext exactly
// (counter-mode XOR is its own inverse; exact equality only holds for padding 0,
// since the padded transform appends crypto/rand filler). The padded (4096)
// decrypter is exercised only for the no-panic guarantee on its length-mismatch
// error branch.
func FuzzDecrypt(f *testing.F) {
	key := encryption.GenerateRandomKey(encryption.KeyLength)
	hashFunc := sha3.NewLegacyKeccak256

	seed := encryption.New(key, 0, 0, hashFunc)
	for _, n := range []int{0, 1, 31, 32, 33, 64, 4096} {
		seed.Reset()
		ct, err := seed.Encrypt(make([]byte, n))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(ct, uint32(0))
	}
	f.Add([]byte{}, uint32(0))
	f.Add([]byte{0x01}, uint32(1))
	f.Add(make([]byte, 4097), uint32(0))

	f.Fuzz(func(t *testing.T, data []byte, initCtr uint32) {
		// (a) padding-free decrypter must not panic and preserves length.
		dec := encryption.New(key, 0, initCtr, hashFunc)
		out, err := dec.Decrypt(data) // must not panic on any input
		if err == nil && len(out) != len(data) {
			t.Fatalf("padding=0 decrypt changed length: in %d out %d", len(data), len(out))
		}

		// (b) round-trip: Decrypt(Encrypt(x)) == x for padding 0.
		enc := encryption.New(key, 0, initCtr, hashFunc)
		ct, err := enc.Encrypt(data)
		if err != nil {
			t.Fatalf("padding=0 encrypt failed: %v", err)
		}
		rt := encryption.New(key, 0, initCtr, hashFunc)
		pt, err := rt.Decrypt(ct)
		if err != nil {
			t.Fatalf("round-trip decrypt failed: %v", err)
		}
		if !bytes.Equal(pt, data) {
			t.Fatalf("round-trip mismatch: in %x out %x", data, pt)
		}

		// (c) padded decrypter exercises the length!=padding error branch;
		// only the no-panic guarantee is asserted here.
		padded := encryption.New(key, swarm.ChunkSize, 0, hashFunc)
		_, _ = padded.Decrypt(data)
	})
}
