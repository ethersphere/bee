// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package store_test

import (
	"encoding/binary"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/encryption"
	"github.com/ethersphere/bee/v2/pkg/encryption/store"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"golang.org/x/crypto/sha3"
)

// keyFromSeed derives a fixed 32-byte encryption key from arbitrary fuzz bytes so
// the key length always matches the hash size and no key-driven behaviour masks
// the parser under test.
func keyFromSeed(seed []byte) encryption.Key {
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write(seed)
	return h.Sum(nil)
}

// encryptChunk builds a valid encrypted chunk (encrypted 8-byte span followed by
// the 4096-byte encrypted data) for the given plaintext data length, matching how
// the store persists encrypted chunk data.
func encryptChunk(seed []byte, dataLen uint64) []byte {
	key := keyFromSeed(seed)
	plain := make([]byte, swarm.SpanSize+swarm.ChunkSize)
	binary.LittleEndian.PutUint64(plain[:swarm.SpanSize], dataLen)

	encSpan, err := encryption.NewSpanEncryption(key).Encrypt(plain[:swarm.SpanSize])
	if err != nil {
		panic(err)
	}
	encData, err := encryption.NewDataEncryption(key).Encrypt(plain[swarm.SpanSize:])
	if err != nil {
		panic(err)
	}
	return append(encSpan, encData...)
}

// FuzzDecryptChunkData drives store.DecryptChunkData over fuzzed persisted chunk
// bytes and a fuzzed key seed. This is the true offset/length-driven parser: it
// slices chunkData[:SpanSize] / chunkData[SpanSize:], decodes
// a redundancy span, and copies decryptedData[:length] where length derives from
// the (decrypted, attacker-influenced) span and may be re-expanded by
// file.ReferenceCount. The parser must never panic on any input.
func FuzzDecryptChunkData(f *testing.F) {
	seedKey := []byte("swarm-fuzz-key")

	f.Add(encryptChunk(seedKey, swarm.ChunkSize), seedKey)
	f.Add(encryptChunk(seedKey, 1), seedKey)
	f.Add(encryptChunk(seedKey, 0), seedKey)
	// exactly SpanSize bytes: span slice succeeds, data decryption errors out.
	f.Add(make([]byte, swarm.SpanSize), seedKey)

	f.Fuzz(func(t *testing.T, chunkData []byte, keySeed []byte) {
		key := keyFromSeed(keySeed)
		// must not panic on any input; an error return is acceptable.
		_, _ = store.DecryptChunkData(chunkData, key)
	})
}
