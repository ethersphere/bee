// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package joiner provides implementations of the file.Joiner interface
package joiner

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/ethersphere/bee/pkg/encryption"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/joiner/internal"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"golang.org/x/crypto/sha3"
)

// simpleJoiner wraps a non-optimized implementation of file.Joiner.
type simpleJoiner struct {
	getter  storage.Getter
	refSize int64
}

// NewSimpleJoiner creates a new simpleJoiner.
func NewSimpleJoiner(getter storage.Getter) file.Joiner {
	return &simpleJoiner{
		getter:  getter,
		refSize: int64(swarm.HashSize),
	}
}

func (s *simpleJoiner) Size(ctx context.Context, address swarm.Address) (dataSize int64, err error) {
	// retrieve the root chunk to read the total data length the be retrieved
	rootChunk, err := s.getter.Get(ctx, storage.ModeGetRequest, address)
	if err != nil {
		return 0, err
	}

	chunkLength := rootChunk.Data()
	if len(chunkLength) < 8 {
		return 0, fmt.Errorf("invalid chunk content of %d bytes", chunkLength)
	}

	dataLength := binary.LittleEndian.Uint64(rootChunk.Data())
	return int64(dataLength), nil
}

// Join implements the file.Joiner interface.
//
// It uses a non-optimized internal component that only retrieves a data chunk
// after the previous has been read.
func (s *simpleJoiner) Join(ctx context.Context, address swarm.Address) (dataOut io.ReadCloser, dataSize int64, err error) {

	// retrieve the root chunk to read the total data length the be retrieved
	rootChunk, err := s.getter.Get(ctx, storage.ModeGetRequest, address)
	if err != nil {
		return nil, 0, err
	}

	// if this is a single chunk, short circuit to returning just that chunk
	spanLength := binary.LittleEndian.Uint64(rootChunk.Data())
	if spanLength <= swarm.ChunkSize {
		data := rootChunk.Data()[8:]
		return file.NewSimpleReadCloser(data), int64(spanLength), nil
	}

	r := internal.NewSimpleJoinerJob(ctx, s.getter, rootChunk)
	return r, int64(spanLength), nil
}

func (s *simpleJoiner) DecryptChunkData(chunkData []byte, encryptionKey encryption.Key) ([]byte, error) {
	if len(chunkData) < 8 {
		return nil, fmt.Errorf("Invalid ChunkData, min length 8 got %v", len(chunkData))
	}

	decryptedSpan, decryptedData, err := s.decrypt(chunkData, encryptionKey)
	if err != nil {
		return nil, err
	}

	// removing extra bytes which were just added for padding
	length := uint64(len(decryptedSpan))
	for length > swarm.ChunkSize {
		length = length + (swarm.ChunkSize - 1)
		length = length / swarm.ChunkSize
		length *= uint64(s.refSize)
	}

	c := make([]byte, length+8)
	copy(c[:8], decryptedSpan)
	copy(c[8:], decryptedData[:length])

	return c, nil
}

func (s *simpleJoiner) decrypt(chunkData []byte, key encryption.Key) ([]byte, []byte, error) {
	encryptedSpan, err := s.newSpanEncryption(key).Encrypt(chunkData[:8])
	if err != nil {
		return nil, nil, err
	}
	encryptedData, err := s.newDataEncryption(key).Encrypt(chunkData[8:])
	if err != nil {
		return nil, nil, err
	}
	return encryptedSpan, encryptedData, nil
}

func (s *simpleJoiner) newSpanEncryption(key encryption.Key) encryption.Encryption {
	return encryption.New(key, 0, uint32(swarm.ChunkSize/s.refSize), sha3.NewLegacyKeccak256)
}

func (s *simpleJoiner) newDataEncryption(key encryption.Key) encryption.Encryption {
	return encryption.New(key, int(swarm.ChunkSize), 0, sha3.NewLegacyKeccak256)
}
