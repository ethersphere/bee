// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package splitter_test

import (
	"context"
	"testing"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	mockbytes "gitlab.com/nolash/go-mockbytes"
)

// TestSplitIncomplete tests that the Split method returns an error if
// the amounts of bytes written does not match the data length passed to the method.
func TestSplitIncomplete(t *testing.T) {
	testData := make([]byte, 42)
	store := mock.NewStorer()
	s := splitter.NewSimpleSplitter(store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testDataReader := file.NewSimpleReadCloser(testData)
	_, err := s.Split(ctx, testDataReader, 41)
	if err == nil {
		t.Fatalf("expected error on EOF before full length write")
	}
}

// TestSplitSingleChunk hashes one single chunk and verifies
// that that corresponding chunk exist in the store afterwards.
func TestSplitSingleChunk(t *testing.T) {

	// edge case selected from internal/job_test.go
	g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
	testData, err := g.SequentialBytes(swarm.ChunkSize)
	if err != nil {
		t.Fatal(err)
	}

	store := mock.NewStorer()
	s := splitter.NewSimpleSplitter(store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testDataReader := file.NewSimpleReadCloser(testData)
	resultAddress, err := s.Split(ctx, testDataReader, int64(len(testData)))
	if err != nil {
		t.Fatal(err)
	}

	testHashHex := "c10090961e7682a10890c334d759a28426647141213abda93b096b892824d2ef"
	testHashAddress := swarm.MustParseHexAddress(testHashHex)
	if !testHashAddress.Equal(resultAddress) {
		t.Fatalf("expected %v, got %v", testHashAddress, resultAddress)
	}

	_, err = store.Get(ctx, storage.ModeGetRequest, resultAddress)
	if err != nil {
		t.Fatal(err)
	}
}

// TestSplitThreeLevels hashes enough data chunks in order to
// create a full chunk of intermediate hashes.
// It verifies that all created chunks exist in the store afterwards.
func TestSplitThreeLevels(t *testing.T) {

	// edge case selected from internal/job_test.go
	g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
	testData, err := g.SequentialBytes(swarm.ChunkSize * swarm.Branches)
	if err != nil {
		t.Fatal(err)
	}

	store := mock.NewStorer()
	s := splitter.NewSimpleSplitter(store)

	//ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond * 500)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testDataReader := file.NewSimpleReadCloser(testData)
	resultAddress, err := s.Split(ctx, testDataReader, int64(len(testData)))
	if err != nil {
		t.Fatal(err)
	}

	testHashHex := "3047d841077898c26bbe6be652a2ec590a5d9bd7cd45d290ea42511b48753c09"
	testHashAddress := swarm.MustParseHexAddress(testHashHex)
	if !testHashAddress.Equal(resultAddress) {
		t.Fatalf("expected %v, got %v", testHashAddress, resultAddress)
	}

	_, err = store.Get(ctx, storage.ModeGetRequest, resultAddress)
	if err != nil {
		t.Fatal(err)
	}

	rootChunk, err := store.Get(ctx, storage.ModeGetRequest, resultAddress)
	if err != nil {
		t.Fatal(err)
	}

	rootData := rootChunk.Data()[8:]
	for i := 0; i < swarm.ChunkSize; i += swarm.SectionSize {
		dataAddressBytes := rootData[i : i+swarm.SectionSize]
		dataAddress := swarm.NewAddress(dataAddressBytes)
		_, err := store.Get(ctx, storage.ModeGetRequest, dataAddress)
		if err != nil {
			t.Fatal(err)
		}
	}
}
