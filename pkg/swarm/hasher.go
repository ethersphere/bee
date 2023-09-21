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

type PrefixHasher struct {
	hash.Hash
	prefix []byte
}

// NewPrefixHasher returns new hasher which is Keccak-256 hasher
// with prefix value added as initial data.
func NewPrefixHasher(prefix []byte) hash.Hash {
	h := &PrefixHasher{
		Hash:   NewHasher(),
		prefix: prefix,
	}
	h.Reset()

	return h
}

func (h *PrefixHasher) Reset() {
	h.Hash.Reset()
	_, _ = h.Write(h.prefix)
}
