// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package joiner_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"testing"

	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/file/pipeline"
	filetest "github.com/ethersphere/bee/pkg/file/testing"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"gitlab.com/nolash/go-mockbytes"
)

// TestJoiner verifies that a newly created joiner returns the data stored
// in the store when the reference is one single chunk.
func TestJoinerSingleChunk(t *testing.T) {
	store := mock.NewStorer()

	joiner := joiner.NewSimpleJoiner(store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var err error
	_, _, err = joiner.Join(ctx, swarm.ZeroAddress, false)
	if err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound for %x", swarm.ZeroAddress)
	}

	// create the chunk to
	mockAddrHex := fmt.Sprintf("%064s", "2a")
	mockAddr := swarm.MustParseHexAddress(mockAddrHex)
	mockData := []byte("foo")
	mockDataLengthBytes := make([]byte, 8)
	mockDataLengthBytes[0] = 0x03
	mockChunk := swarm.NewChunk(mockAddr, append(mockDataLengthBytes, mockData...))
	_, err = store.Put(ctx, storage.ModePutUpload, mockChunk)
	if err != nil {
		t.Fatal(err)
	}

	// read back data and compare
	joinReader, l, err := joiner.Join(ctx, mockAddr, false)
	if err != nil {
		t.Fatal(err)
	}
	if l != int64(len(mockData)) {
		t.Fatalf("expected join data length %d, got %d", len(mockData), l)
	}
	joinData, err := ioutil.ReadAll(joinReader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(joinData, mockData) {
		t.Fatalf("retrieved data '%x' not like original data '%x'", joinData, mockData)
	}
}

// TestJoinerWithReference verifies that a chunk reference is correctly resolved
// and the underlying data is returned.
func TestJoinerWithReference(t *testing.T) {
	store := mock.NewStorer()
	joiner := joiner.NewSimpleJoiner(store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// create root chunk and two data chunks referenced in the root chunk
	rootChunk := filetest.GenerateTestRandomFileChunk(swarm.ZeroAddress, swarm.ChunkSize*2, swarm.SectionSize*2)
	_, err := store.Put(ctx, storage.ModePutUpload, rootChunk)
	if err != nil {
		t.Fatal(err)
	}

	firstAddress := swarm.NewAddress(rootChunk.Data()[8 : swarm.SectionSize+8])
	firstChunk := filetest.GenerateTestRandomFileChunk(firstAddress, swarm.ChunkSize, swarm.ChunkSize)
	_, err = store.Put(ctx, storage.ModePutUpload, firstChunk)
	if err != nil {
		t.Fatal(err)
	}

	secondAddress := swarm.NewAddress(rootChunk.Data()[swarm.SectionSize+8:])
	secondChunk := filetest.GenerateTestRandomFileChunk(secondAddress, swarm.ChunkSize, swarm.ChunkSize)
	_, err = store.Put(ctx, storage.ModePutUpload, secondChunk)
	if err != nil {
		t.Fatal(err)
	}

	// read back data and compare
	joinReader, l, err := joiner.Join(ctx, rootChunk.Address(), false)
	if err != nil {
		t.Fatal(err)
	}
	if l != int64(swarm.ChunkSize*2) {
		t.Fatalf("expected join data length %d, got %d", swarm.ChunkSize*2, l)
	}

	resultBuffer := make([]byte, swarm.ChunkSize)
	n, err := joinReader.Read(resultBuffer)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(resultBuffer) {
		t.Fatalf("expected read count %d, got %d", len(resultBuffer), n)
	}
	if !bytes.Equal(resultBuffer, firstChunk.Data()[8:]) {
		t.Fatalf("expected resultbuffer %v, got %v", resultBuffer, firstChunk.Data()[:len(resultBuffer)])
	}
}

func TestEncryptionAndDecryption(t *testing.T) {
	var tests = []struct {
		chunkLength int
	}{
		{10},
		{100},
		{1000},
		{4095},
		{4096},
		{4097},
		{15000},
		{256 * 1024},
		// {256 * 1024 * 2}, // TODO: fix - incorrect join
		// {256 * 1024 * 3}, // TODO: fix - deadlock
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Encrypt %d bytes", tt.chunkLength), func(t *testing.T) {
			store := mock.NewStorer()
			joinner := joiner.NewSimpleJoiner(store)

			g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
			testData, err := g.SequentialBytes(tt.chunkLength)
			if err != nil {
				t.Fatal(err)
			}

			ctx := context.Background()
			pipe := pipeline.NewEncryptionPipeline(ctx, store, storage.ModePutUpload)
			testDataReader := bytes.NewReader(testData)
			resultAddress, err := pipeline.FeedPipeline(ctx, pipe, testDataReader, int64(len(testData)))
			if err != nil {
				t.Fatal(err)
			}

			reader, l, err := joinner.Join(context.Background(), resultAddress, true)
			if err != nil {
				t.Fatal(err)
			}

			if l != int64(len(testData)) {
				t.Fatalf("expected join data length %d, got %d", len(testData), l)
			}

			totalGot := make([]byte, tt.chunkLength)
			index := 0
			resultBuffer := make([]byte, swarm.ChunkSize)

			for index < tt.chunkLength {
				n, err := reader.Read(resultBuffer)
				if err != nil && err != io.EOF {
					t.Fatal(err)
				}
				copy(totalGot[index:], resultBuffer[:n])
				index += n
			}

			if !bytes.Equal(testData, totalGot) {
				t.Fatal("input data and output data does not match")
			}
		})
	}
}
