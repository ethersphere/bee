// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package encryption

import (
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"golang.org/x/crypto/sha3"
)

// ChunkEncrypter encrypts chunk data.
type ChunkEncrypter interface {
	EncryptChunk([]byte) (key Key, encryptedSpan, encryptedData []byte, err error)
}

type chunkEncrypter struct{}

func NewChunkEncrypter() ChunkEncrypter { return &chunkEncrypter{} }

func (c *chunkEncrypter) EncryptChunk(chunkData []byte) (Key, []byte, []byte, error) {
	key := GenerateRandomKey(KeyLength)
	encryptedSpan, err := NewSpanEncryption(key).Encrypt(chunkData[:8])
	if err != nil {
		return nil, nil, nil, err
	}
	encryptedData, err := NewDataEncryption(key).Encrypt(chunkData[8:])
	if err != nil {
		return nil, nil, nil, err
	}
	return key, encryptedSpan, encryptedData, nil
}

func NewSpanEncryption(key Key) Interface {
	return New(key, 0, uint32(swarm.ChunkSize/KeyLength), sha3.NewLegacyKeccak256)
}

func NewDataEncryption(key Key) Interface {
	return New(key, int(swarm.ChunkSize), 0, sha3.NewLegacyKeccak256)
}
