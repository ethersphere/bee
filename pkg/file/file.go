// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package file provides interfaces for file-oriented operations.
package file

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/ethersphere/bee/pkg/encryption"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/swarm/chunk"
	"golang.org/x/crypto/sha3"
)

var (
	ChunkWithLengthSize = swarm.ChunkSize + 8
)

// Joiner returns file data referenced by the given Swarm Address to the given io.Reader.
//
// The call returns when the chunk for the given Swarm Address is found,
// returning the length of the data which will be returned.
// The called can then read the data on the io.Reader that was provided.
type Joiner interface {
	Join(ctx context.Context, address swarm.Address) (dataOut io.ReadCloser, dataLength int64, err error)
	Size(ctx context.Context, address swarm.Address) (dataLength int64, err error)
}

// Splitter starts a new file splitting job.
//
// Data is read from the provided reader.
// If the dataLength parameter is 0, data is read until io.EOF is encountered.
// When EOF is received and splitting is done, the resulting Swarm Address is returned.
type Splitter interface {
	Split(ctx context.Context, dataIn io.ReadCloser, dataLength int64, toEncrypt bool) (addr swarm.Address, err error)
}

// JoinReadAll reads all output from the provided joiner.
func JoinReadAll(j Joiner, addr swarm.Address, outFile io.Writer) (int64, error) {
	r, l, err := j.Join(context.Background(), addr)
	if err != nil {
		return 0, err
	}
	// join, rinse, repeat until done
	data := make([]byte, swarm.ChunkSize)
	var total int64
	for i := int64(0); i < l; i += swarm.ChunkSize {
		cr, err := r.Read(data)
		if err != nil {
			return total, err
		}
		total += int64(cr)
		cw, err := outFile.Write(data[:cr])
		if err != nil {
			return total, err
		}
		if cw != cr {
			return total, fmt.Errorf("short wrote %d of %d for chunk %d", cw, cr, i)
		}
	}
	if total != l {
		return total, fmt.Errorf("received only %d of %d total bytes", total, l)
	}
	return total, nil
}

func DecryptChunkData(chunkData []byte, encryptionKey encryption.Key) ([]byte, error) {
	if len(chunkData) < 8 {
		return nil, fmt.Errorf("Invalid ChunkData, min length 8 got %v", len(chunkData))
	}

	decryptedSpan, decryptedData, err := decrypt(chunkData, encryptionKey)
	if err != nil {
		return nil, err
	}

	// removing extra bytes which were just added for padding
	length := binary.LittleEndian.Uint64(decryptedSpan)
	//length := uint64(len(decryptedSpan))
	for length > chunk.DefaultSize {
		length = length + (chunk.DefaultSize - 1)
		length = length / chunk.DefaultSize
		length *= uint64(swarm.HashSize + encryption.KeyLength)
	}

	c := make([]byte, length+8)
	copy(c[:8], decryptedSpan)
	copy(c[8:], decryptedData[:length])

	return c, nil
}

func decrypt(chunkData []byte, key encryption.Key) ([]byte, []byte, error) {
	encryptedSpan, err := newSpanEncryption(key).Encrypt(chunkData[:8])
	if err != nil {
		return nil, nil, err
	}
	encryptedData, err := newDataEncryption(key).Encrypt(chunkData[8:])
	if err != nil {
		return nil, nil, err
	}
	return encryptedSpan, encryptedData, nil
}

func newSpanEncryption(key encryption.Key) encryption.Encryption {
	return encryption.New(key, 0, uint32(chunk.DefaultSize/uint64(swarm.HashSize+encryption.KeyLength)), sha3.NewLegacyKeccak256)
}

func newDataEncryption(key encryption.Key) encryption.Encryption {
	return encryption.New(key, int(chunk.DefaultSize), 0, sha3.NewLegacyKeccak256)
}
