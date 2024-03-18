// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package store

import (
	"context"
	"encoding/binary"

	"github.com/ethersphere/bee/v2/pkg/encryption"
	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type decryptingStore struct {
	storage.Getter
}

func New(s storage.Getter) storage.Getter {
	return &decryptingStore{s}
}

func (s *decryptingStore) Get(ctx context.Context, addr swarm.Address) (ch swarm.Chunk, err error) {
	switch l := len(addr.Bytes()); l {
	case swarm.HashSize:
		// normal, unencrypted content
		return s.Getter.Get(ctx, addr)

	case encryption.ReferenceSize:
		// encrypted reference
		ref := addr.Bytes()
		address := swarm.NewAddress(ref[:swarm.HashSize])
		ch, err := s.Getter.Get(ctx, address)
		if err != nil {
			return nil, err
		}

		d, err := DecryptChunkData(ch.Data(), ref[swarm.HashSize:])
		if err != nil {
			return nil, err
		}
		return swarm.NewChunk(address, d), nil

	default:
		return nil, storage.ErrReferenceLength
	}
}

func DecryptChunkData(chunkData []byte, encryptionKey encryption.Key) ([]byte, error) {
	decryptedSpan, decryptedData, err := decrypt(chunkData, encryptionKey)
	if err != nil {
		return nil, err
	}

	// removing extra bytes which were just added for padding
	level, span := redundancy.DecodeSpan(decryptedSpan)
	length := binary.LittleEndian.Uint64(span)
	if length > swarm.ChunkSize {
		dataRefSize := uint64(swarm.HashSize + encryption.KeyLength)
		dataShards, parities := file.ReferenceCount(length, level, true)
		length = dataRefSize*uint64(dataShards) + uint64(parities*swarm.HashSize)
	}

	c := make([]byte, length+8)
	copy(c[:8], decryptedSpan)
	copy(c[8:], decryptedData[:length])

	return c, nil
}

func decrypt(chunkData []byte, key encryption.Key) ([]byte, []byte, error) {
	decryptedSpan, err := encryption.NewSpanEncryption(key).Decrypt(chunkData[:swarm.SpanSize])
	if err != nil {
		return nil, nil, err
	}
	decryptedData, err := encryption.NewDataEncryption(key).Decrypt(chunkData[swarm.SpanSize:])
	if err != nil {
		return nil, nil, err
	}
	return decryptedSpan, decryptedData, nil
}
