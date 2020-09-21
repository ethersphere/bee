// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package encryption_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	mockenc "github.com/ethersphere/bee/pkg/encryption/mock"
	"github.com/ethersphere/bee/pkg/file/pipeline"
	"github.com/ethersphere/bee/pkg/file/pipeline/encryption"
	mock "github.com/ethersphere/bee/pkg/file/pipeline/mock"
)

var (
	addr                         = []byte{0xaa, 0xbb, 0xcc}
	key                          = []byte("abcd")
	data                         = []byte("hello world")
	encryptedSpan, encryptedData []byte
)

func init() {
	mockEncrypter := mockenc.New(mockenc.WithXOREncryption(key))
	var err error
	encryptedData, err = mockEncrypter.Encrypt(data)
	if err != nil {
		panic(err)
	}
	span := make([]byte, 8)
	binary.BigEndian.PutUint64(span, uint64(len(data)))
	encryptedSpan, err = mockEncrypter.Encrypt(span)
	if err != nil {
		panic(err)
	}
}

// TestEncyrption tests that the encyption writer works correctly.
func TestEncryption(t *testing.T) {
	mockChainWriter := mock.NewChainWriter()
	writer := encryption.NewEncryptionWriter(mockenc.NewChunkEncrypter(key), mockChainWriter)

	args := pipeline.PipeWriteArgs{Ref: addr, Data: data}
	err := writer.ChainWrite(&args)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encryptedData, args.Data) {
		t.Fatalf("data mismatch. got %v want %v", args.Data, encryptedData)
	}

	if calls := mockChainWriter.ChainWriteCalls(); calls != 1 {
		t.Errorf("wanted 1 ChainWrite call, got %d", calls)
	}
}

// TestSum tests that calling Sum on the store writer results in Sum on the next writer in the chain.
func TestSum(t *testing.T) {
	mockChainWriter := mock.NewChainWriter()
	writer := encryption.NewEncryptionWriter(nil, mockChainWriter)

	_, err := writer.Sum()
	if err != nil {
		t.Fatal(err)
	}
	if calls := mockChainWriter.SumCalls(); calls != 1 {
		t.Fatalf("wanted 1 Sum call but got %d", calls)
	}
}
