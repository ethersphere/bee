// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"hash"

	"golang.org/x/crypto/sha3"
)

// NewHasher returns new Keccak-256 hasher.
func NewHasher() hash.Hash {
	return sha3.NewLegacyKeccak256()
}

// NewStatefulHasher returns new Keccak-256 hasher with state modifiers.
func NewStatefulHasher(state []byte) (sha3.StatefulHash, error) {
	return sha3.NewLegacyKeccak256WithState(state)
}

type PrefixHasher struct {
	sha3.StatefulHash
	prefix       []byte
	initialState []byte
}

// NewPrefixHasher returns new hasher which is Keccak-256 hasher
// with prefix value added as state.
func NewPrefixHasher(prefix []byte) hash.Hash {
	h, err := NewStatefulHasher(nil)
	if err != nil {
		return nil
	}
	_, _ = h.Write(prefix)

	return &PrefixHasher{
		StatefulHash: h,
		prefix:       prefix,
		initialState: h.ExportState(),
	}
}

func (h *PrefixHasher) Reset() {
	_ = h.ImportState(h.initialState)
}
