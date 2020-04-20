// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package joiner_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestJoiner verifies that a newly created joiner
// returns the data stored in the store for a given reference
func TestJoiner(t *testing.T) {
	store := mock.NewStorer()

	joiner := joiner.NewSimpleJoiner(store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var err error
	_, _, err = joiner.Join(ctx, swarm.ZeroAddress)
	if err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound for %x", swarm.ZeroAddress)
	}

	mockAddrHex := fmt.Sprintf("%064s", "2a")
	mockAddr := swarm.MustParseHexAddress(mockAddrHex)
	mockData := []byte("foo")
	mockDataLengthBytes := make([]byte, 8)
	mockDataLengthBytes[0] = 0x03;
	mockChunk := swarm.NewChunk(mockAddr, append(mockDataLengthBytes, mockData...))
	_, err = store.Put(ctx, storage.ModePutRequest, mockChunk)
	if err != nil {
		t.Fatal(err)
	}

	joinReader, l, err := joiner.Join(ctx, mockAddr)
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
// and the underlying data is returned
func TestJoinerWithReference(t *testing.T) {
	store := mock.NewStorer()
	joiner := joiner.NewSimpleJoiner(store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var err error
	_, _, err = joiner.Join(ctx, swarm.ZeroAddress)
	if err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound for %x", swarm.ZeroAddress)
	}

	rootAddrHex := fmt.Sprintf("%064s", "2a")
	rootAddr := swarm.MustParseHexAddress(rootAddrHex)
	firstAddrHex := fmt.Sprintf("%064s", "01a6")
	firstAddr := swarm.MustParseHexAddress(firstAddrHex)
	secondAddrHex := fmt.Sprintf("%064s", "029a")
	secondAddr := swarm.MustParseHexAddress(secondAddrHex)

	firstData := make([]byte, swarm.ChunkSize)
	copy(firstData, []byte("foo"))
	secondData := []byte("bar")

	firstLength := len(firstData)
	firstLengthBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(firstLengthBytes, uint64(firstLength))

	secondLength := len(secondData)
	secondLengthBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(secondLengthBytes, uint64(secondLength))

	totalLength := firstLength + secondLength
	totalLengthBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(totalLengthBytes, uint64(totalLength))

	rootData := append(firstAddr.Bytes(), secondAddr.Bytes()...)
	rootChunk := swarm.NewChunk(rootAddr, append(totalLengthBytes, rootData...))
	_, err = store.Put(ctx, storage.ModePutRequest, rootChunk)

	firstChunk := swarm.NewChunk(firstAddr, append(firstLengthBytes, firstData...))
	_, err = store.Put(ctx, storage.ModePutRequest, firstChunk)

	secondChunk := swarm.NewChunk(secondAddr, append(secondLengthBytes, secondData...))
	_, err = store.Put(ctx, storage.ModePutRequest, secondChunk)

	joinReader, l, err := joiner.Join(ctx, rootAddr)
	if l != int64(len(firstData) + len(secondData)) {
		t.Fatalf("expected join data length %d, got %d", len(firstData) + len(secondData), l)
	}

	resultBuffer := make([]byte, 3)
	n, err := joinReader.Read(resultBuffer)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(resultBuffer) {
		t.Fatalf("expected read count %d, got %d", len(resultBuffer), n)
	}
	if !bytes.Equal(resultBuffer, firstData[:len(resultBuffer)]) {
		t.Fatalf("expected resultbuffer %v, got %v", resultBuffer, firstData[:len(resultBuffer)])
	}
}
